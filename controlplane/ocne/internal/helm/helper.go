package helm

import (
	"bytes"
	"context"
	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
)

func initializeBuiltins(ctx context.Context, c ctrlClient.Client, referenceMap map[string]corev1.ObjectReference, cluster *clusterv1.Cluster) (map[string]interface{}, error) {
	log := ctrl.LoggerFrom(ctx)

	valueLookUp := make(map[string]interface{})

	for name, ref := range referenceMap {
		log.V(2).Info("Getting object for reference", "ref", ref)
		obj, err := external.Get(ctx, c, &ref, cluster.Namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get object %s", ref.Name)
		}
		valueLookUp[name] = obj.Object
	}

	return valueLookUp, nil
}

// ParseValues parses the values template and returns the expanded template. It attempts to populate a map of supported templating objects.
func ParseValuesTemplate(ctx context.Context, c ctrlClient.Client, spec bootstrapv1.ModuleAddons, cluster *clusterv1.Cluster) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Rendering templating in values:", "values", spec.ValuesTemplate)
	references := map[string]corev1.ObjectReference{
		"Cluster": {
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Namespace:  cluster.Namespace,
			Name:       cluster.Name,
		},
	}

	if cluster.Spec.ControlPlaneRef != nil {
		references["ControlPlane"] = *cluster.Spec.ControlPlaneRef
	}
	if cluster.Spec.InfrastructureRef != nil {
		references["InfraCluster"] = *cluster.Spec.InfrastructureRef
	}
	// TODO: would we want to add ControlPlaneMachineTemplate?

	valueLookUp, err := initializeBuiltins(ctx, c, references, cluster)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(spec.ChartName + "-" + cluster.GetName()).
		Funcs(sprig.TxtFuncMap()).
		Parse(spec.ValuesTemplate)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer

	if err := tmpl.Execute(&buffer, valueLookUp); err != nil {
		return "", errors.Wrapf(err, "error executing template string '%s' on cluster '%s'", spec.ValuesTemplate, cluster.GetName())
	}
	expandedTemplate := buffer.String()
	log.V(2).Info("Expanded values to", "result", expandedTemplate)

	return expandedTemplate, nil
}
