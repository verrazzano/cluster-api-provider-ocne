/*
Copyright 2021 The Kubernetes Authors.

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

package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// Conditions and condition Reasons for the OCNEControlPlane object.

const (
	// MachinesReadyCondition reports an aggregate of current status of the machines controlled by the OCNEControlPlane.
	MachinesReadyCondition clusterv1.ConditionType = "MachinesReady"
)

const (
	// CertificatesAvailableCondition documents that cluster certificates were generated as part of the
	// processing of a OCNEControlPlane object.
	CertificatesAvailableCondition clusterv1.ConditionType = "CertificatesAvailable"

	// CertificatesGenerationFailedReason (Severity=Warning) documents a OCNEControlPlane controller detecting
	// an error while generating certificates; those kind of errors are usually temporary and the controller
	// automatically recover from them.
	CertificatesGenerationFailedReason = "CertificatesGenerationFailed"
)

const (
	// AvailableCondition documents that the first control plane instance has completed the ocne init operation
	// and so the control plane is available and an API server instance is ready for processing requests.
	AvailableCondition clusterv1.ConditionType = "Available"

	// WaitingForOCNEInitReason (Severity=Info) documents a OCNEControlPlane object waiting for the first
	// control plane instance to complete the ocne init operation.
	WaitingForOCNEInitReason = "WaitingForOCNEInit"
)

const (
	// MachinesSpecUpToDateCondition documents that the spec of the machines controlled by the OCNEControlPlane
	// is up to date. When this condition is false, the OCNEControlPlane is executing a rolling upgrade.
	MachinesSpecUpToDateCondition clusterv1.ConditionType = "MachinesSpecUpToDate"

	// RollingUpdateInProgressReason (Severity=Warning) documents a OCNEControlPlane object executing a
	// rolling upgrade for aligning the machines spec to the desired state.
	RollingUpdateInProgressReason = "RollingUpdateInProgress"
)

const (
	// ResizedCondition documents a OCNEControlPlane that is resizing the set of controlled machines.
	ResizedCondition clusterv1.ConditionType = "Resized"

	// ScalingUpReason (Severity=Info) documents a OCNEControlPlane that is increasing the number of replicas.
	ScalingUpReason = "ScalingUp"

	// ScalingDownReason (Severity=Info) documents a OCNEControlPlane that is decreasing the number of replicas.
	ScalingDownReason = "ScalingDown"
)

const (
	// ModuleOperatorDeploy reports that OCNEModuleOperator is deployed
	ModuleOperatorDeploy clusterv1.ConditionType = "OCNEModuleOperatorDeployed"

	// ModuleOperatorUninstalled (Severity=Info) documents a OCNEControlPlane that OCNEModule operator is uninstalled.
	ModuleOperatorUninstalled string = "ModuleOperatorUninstalled"
)

const (
	// ControlPlaneComponentsHealthyCondition reports the overall status of control plane components
	// implemented as static pods generated by ocne including kube-api-server, kube-controller manager,
	// kube-scheduler and etcd if managed.
	ControlPlaneComponentsHealthyCondition clusterv1.ConditionType = "ControlPlaneComponentsHealthy"

	// ControlPlaneComponentsUnhealthyReason (Severity=Error) documents a control plane component not healthy.
	ControlPlaneComponentsUnhealthyReason = "ControlPlaneComponentsUnhealthy"

	// ControlPlaneComponentsUnknownReason reports a control plane component in unknown status.
	ControlPlaneComponentsUnknownReason = "ControlPlaneComponentsUnknown"

	// ControlPlaneComponentsInspectionFailedReason documents a failure in inspecting the control plane component status.
	ControlPlaneComponentsInspectionFailedReason = "ControlPlaneComponentsInspectionFailed"

	// MachineAPIServerPodHealthyCondition reports a machine's kube-apiserver's operational status.
	MachineAPIServerPodHealthyCondition clusterv1.ConditionType = "APIServerPodHealthy"

	// MachineControllerManagerPodHealthyCondition reports a machine's kube-controller-manager's health status.
	MachineControllerManagerPodHealthyCondition clusterv1.ConditionType = "ControllerManagerPodHealthy"

	// MachineSchedulerPodHealthyCondition reports a machine's kube-scheduler's operational status.
	MachineSchedulerPodHealthyCondition clusterv1.ConditionType = "SchedulerPodHealthy"

	// MachineEtcdPodHealthyCondition reports a machine's etcd pod's operational status.
	// NOTE: This conditions exists only if a stacked etcd cluster is used.
	MachineEtcdPodHealthyCondition clusterv1.ConditionType = "EtcdPodHealthy"

	// PodProvisioningReason (Severity=Info) documents a pod waiting to be provisioned i.e., Pod is in "Pending" phase.
	PodProvisioningReason = "PodProvisioning"

	// PodMissingReason (Severity=Error) documents a pod does not exist.
	PodMissingReason = "PodMissing"

	// PodFailedReason (Severity=Error) documents if a pod failed during provisioning i.e., e.g CrashLoopbackOff, ImagePullBackOff
	// or if all the containers in a pod have terminated.
	PodFailedReason = "PodFailed"

	// PodInspectionFailedReason documents a failure in inspecting the pod status.
	PodInspectionFailedReason = "PodInspectionFailed"
)

const (
	// EtcdClusterHealthyCondition documents the overall etcd cluster's health.
	EtcdClusterHealthyCondition clusterv1.ConditionType = "EtcdClusterHealthy"

	// EtcdClusterInspectionFailedReason documents a failure in inspecting the etcd cluster status.
	EtcdClusterInspectionFailedReason = "EtcdClusterInspectionFailed"

	// EtcdClusterUnknownReason reports an etcd cluster in unknown status.
	EtcdClusterUnknownReason = "EtcdClusterUnknown"

	// EtcdClusterUnhealthyReason (Severity=Error) is set when the etcd cluster is unhealthy.
	EtcdClusterUnhealthyReason = "EtcdClusterUnhealthy"

	// MachineEtcdMemberHealthyCondition report the machine's etcd member's health status.
	// NOTE: This conditions exists only if a stacked etcd cluster is used.
	MachineEtcdMemberHealthyCondition clusterv1.ConditionType = "EtcdMemberHealthy"

	// EtcdMemberInspectionFailedReason documents a failure in inspecting the etcd member status.
	EtcdMemberInspectionFailedReason = "MemberInspectionFailed"

	// EtcdMemberUnhealthyReason (Severity=Error) documents a Machine's etcd member is unhealthy.
	EtcdMemberUnhealthyReason = "EtcdMemberUnhealthy"

	// MachinesCreatedCondition documents that the machines controlled by the OCNEControlPlane are created.
	// When this condition is false, it indicates that there was an error when cloning the infrastructure/bootstrap template or
	// when generating the machine object.
	MachinesCreatedCondition clusterv1.ConditionType = "MachinesCreated"

	// InfrastructureTemplateCloningFailedReason (Severity=Error) documents a OCNEControlPlane failing to
	// clone the infrastructure template.
	InfrastructureTemplateCloningFailedReason = "InfrastructureTemplateCloningFailed"

	// BootstrapTemplateCloningFailedReason (Severity=Error) documents a OCNEControlPlane failing to
	// clone the bootstrap template.
	BootstrapTemplateCloningFailedReason = "BootstrapTemplateCloningFailed"

	// MachineGenerationFailedReason (Severity=Error) documents a OCNEControlPlane failing to
	// generate a machine object.
	MachineGenerationFailedReason = "MachineGenerationFailed"
)
