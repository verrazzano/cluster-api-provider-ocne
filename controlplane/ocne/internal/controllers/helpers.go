/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file from the cluster-api community (https://github.com/kubernetes-sigs/cluster-api) has been modified by Oracle.

package controllers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/certs"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	utilconversion "github.com/verrazzano/cluster-api-provider-ocne/util/conversion"
	"github.com/verrazzano/cluster-api-provider-ocne/util/kubeconfig"
	"github.com/verrazzano/cluster-api-provider-ocne/util/patch"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
)

func (r *OCNEControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	endpoint := cluster.Spec.ControlPlaneEndpoint
	if endpoint.IsZero() {
		return ctrl.Result{}, nil
	}

	controllerOwnerRef := *metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))
	clusterName := util.ObjectKey(cluster)
	configSecret, err := secret.GetFromNamespacedName(ctx, r.Client, clusterName, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			controllerOwnerRef,
		)
		if errors.Is(createErr, kubeconfig.ErrDependentCertificateNotFound) {
			return ctrl.Result{RequeueAfter: dependentCertRequeueAfter}, nil
		}
		// always return if we have just created in order to skip rotation checks
		return ctrl.Result{}, createErr
	case err != nil:
		return ctrl.Result{}, errors.Wrap(err, "failed to retrieve kubeconfig Secret")
	}

	if err := r.adoptKubeconfigSecret(ctx, cluster, configSecret, ocnecp); err != nil {
		return ctrl.Result{}, err
	}

	// only do rotation on owned secrets
	if !util.IsControlledBy(configSecret, ocnecp) {
		return ctrl.Result{}, nil
	}

	needsRotation, err := kubeconfig.NeedsClientCertRotation(configSecret, certs.ClientCertificateRenewalDuration)
	if err != nil {
		return ctrl.Result{}, err
	}

	if needsRotation {
		log.Info("rotating kubeconfig secret")
		if err := kubeconfig.RegenerateSecret(ctx, r.Client, configSecret); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to regenerate kubeconfig")
		}
	}

	return ctrl.Result{}, nil
}

// Ensure the KubeadmConfigSecret has an owner reference to the control plane if it is not a user-provided secret.
func (r *OCNEControlPlaneReconciler) adoptKubeconfigSecret(ctx context.Context, cluster *clusterv1.Cluster, configSecret *corev1.Secret, ocnecp *controlplanev1.OCNEControlPlane) error {
	log := ctrl.LoggerFrom(ctx)
	controller := metav1.GetControllerOf(configSecret)

	// If the Type doesn't match the CAPI-created secret type this is a no-op.
	if configSecret.Type != clusterv1.ClusterSecretType {
		return nil
	}
	// If the secret is already controlled by KCP this is a no-op.
	if controller != nil && controller.Kind == "OCNEControlPlane" {
		return nil
	}
	log.Info("Adopting KubeConfig secret", "Secret", klog.KObj(configSecret))
	patch, err := patch.NewHelper(configSecret, r.Client)
	if err != nil {
		return errors.Wrap(err, "failed to create patch helper for the kubeconfig secret")
	}

	// If the kubeconfig secret was created by v1alpha2 controllers, and thus it has the Cluster as the owner instead of KCP.
	// In this case remove the ownerReference to the Cluster.
	if util.IsOwnedByObject(configSecret, cluster) {
		configSecret.SetOwnerReferences(util.RemoveOwnerRef(configSecret.OwnerReferences, metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))
	}

	// Remove the current controller if one exists.
	if controller != nil {
		configSecret.SetOwnerReferences(util.RemoveOwnerRef(configSecret.OwnerReferences, *controller))
	}

	// Add the OCNEControlPlane as the controller for this secret.
	configSecret.OwnerReferences = util.EnsureOwnerRef(configSecret.OwnerReferences,
		*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane")))

	if err := patch.Patch(ctx, configSecret); err != nil {
		return errors.Wrap(err, "failed to patch the kubeconfig secret")
	}
	return nil
}

func (r *OCNEControlPlaneReconciler) reconcileExternalReference(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, r.APIReader, ref); err != nil {
		return err
	}

	obj, err := external.Get(ctx, r.Client, ref, cluster.Namespace)
	if err != nil {
		return err
	}

	// Note: We intentionally do not handle checking for the paused label on an external template reference

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	return patchHelper.Patch(ctx, obj)
}

func (r *OCNEControlPlaneReconciler) cloneConfigsAndGenerateMachine(ctx context.Context, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane, bootstrapSpec *bootstrapv1.OCNEConfigSpec, failureDomain *string) error {
	var errs []error

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "OCNEControlPlane",
		Name:       ocnecp.Name,
		UID:        ocnecp.UID,
	}

	// Clone the infrastructure template
	infraRef, err := external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
		Client:      r.Client,
		TemplateRef: &ocnecp.Spec.MachineTemplate.InfrastructureRef,
		Namespace:   ocnecp.Namespace,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
		Annotations: ocnecp.Spec.MachineTemplate.ObjectMeta.Annotations,
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		conditions.MarkFalse(ocnecp, controlplanev1.MachinesCreatedCondition, controlplanev1.InfrastructureTemplateCloningFailedReason,
			clusterv1.ConditionSeverityError, err.Error())
		return errors.Wrap(err, "failed to clone infrastructure template")
	}

	// Clone the bootstrap configuration
	bootstrapRef, err := r.generateKubeadmConfig(ctx, ocnecp, cluster, bootstrapSpec)
	if err != nil {
		conditions.MarkFalse(ocnecp, controlplanev1.MachinesCreatedCondition, controlplanev1.BootstrapTemplateCloningFailedReason,
			clusterv1.ConditionSeverityError, err.Error())
		errs = append(errs, errors.Wrap(err, "failed to generate bootstrap config"))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		if err := r.generateMachine(ctx, ocnecp, cluster, infraRef, bootstrapRef, failureDomain); err != nil {
			conditions.MarkFalse(ocnecp, controlplanev1.MachinesCreatedCondition, controlplanev1.MachineGenerationFailedReason,
				clusterv1.ConditionSeverityError, err.Error())
			errs = append(errs, errors.Wrap(err, "failed to create Machine"))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraRef, bootstrapRef); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources"))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *OCNEControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, remoteRefs ...*corev1.ObjectReference) error {
	var errs []error

	for _, ref := range remoteRefs {
		if ref == nil {
			continue
		}
		config := &unstructured.Unstructured{}
		config.SetKind(ref.Kind)
		config.SetAPIVersion(ref.APIVersion)
		config.SetNamespace(ref.Namespace)
		config.SetName(ref.Name)

		if err := r.Client.Delete(ctx, config); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources after error"))
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *OCNEControlPlaneReconciler) generateKubeadmConfig(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane, cluster *clusterv1.Cluster, spec *bootstrapv1.OCNEConfigSpec) (*corev1.ObjectReference, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "OCNEControlPlane",
		Name:       ocnecp.Name,
		UID:        ocnecp.UID,
	}

	bootstrapConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(ocnecp.Name + "-"),
			Namespace:       ocnecp.Namespace,
			Labels:          internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
			Annotations:     ocnecp.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "OCNEConfig",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}

func (r *OCNEControlPlaneReconciler) generateMachine(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane, cluster *clusterv1.Cluster, infraRef, bootstrapRef *corev1.ObjectReference, failureDomain *string) error {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        names.SimpleNameGenerator.GenerateName(ocnecp.Name + "-"),
			Namespace:   ocnecp.Namespace,
			Labels:      internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
			Annotations: map[string]string{},
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           &ocnecp.Spec.Version,
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain:    failureDomain,
			NodeDrainTimeout: ocnecp.Spec.MachineTemplate.NodeDrainTimeout,
		},
	}
	if ocnecp.Spec.MachineTemplate.NodeDeletionTimeout != nil {
		machine.Spec.NodeDeletionTimeout = ocnecp.Spec.MachineTemplate.NodeDeletionTimeout
	}

	// Machine's bootstrap config may be missing ClusterConfiguration if it is not the first machine in the control plane.
	// We store ClusterConfiguration as annotation here to detect any changes in KCP ClusterConfiguration and rollout the machine if any.
	clusterConfig, err := json.Marshal(ocnecp.Spec.OCNEConfigSpec.ClusterConfiguration)
	if err != nil {
		return errors.Wrap(err, "failed to marshal cluster configuration")
	}

	// Add the annotations from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in KCP.
	for k, v := range ocnecp.Spec.MachineTemplate.ObjectMeta.Annotations {
		machine.Annotations[k] = v
	}
	machine.Annotations[controlplanev1.OCNEClusterConfigurationAnnotation] = string(clusterConfig)

	if err := r.Client.Create(ctx, machine); err != nil {
		return errors.Wrap(err, "failed to create machine")
	}
	return nil
}
