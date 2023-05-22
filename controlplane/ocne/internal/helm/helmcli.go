package helm

import (
	"context"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	helmAction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	helmLoader "helm.sh/helm/v3/pkg/chart/loader"
	helmCli "helm.sh/helm/v3/pkg/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"

	helmVals "helm.sh/helm/v3/pkg/cli/values"
	helmGetter "helm.sh/helm/v3/pkg/getter"
	helmRelease "helm.sh/helm/v3/pkg/release"
	helmDriver "helm.sh/helm/v3/pkg/storage/driver"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
)

func GetActionConfig(ctx context.Context, namespace string, config *rest.Config) (*helmAction.Configuration, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Getting action config")
	actionConfig := new(helmAction.Configuration)
	insecure := true
	cliConfig := genericclioptions.NewConfigFlags(false)
	cliConfig.APIServer = &config.Host
	cliConfig.BearerToken = &config.BearerToken
	cliConfig.Namespace = &namespace
	cliConfig.Insecure = &insecure
	// Drop their rest.Config and just return inject own
	wrapper := func(*rest.Config) *rest.Config {
		return config
	}
	cliConfig.WithWrapConfigFn(wrapper)
	// cliConfig.Insecure = &insecure
	// Note: can change this to klog.V(4) or use a debug level
	if err := actionConfig.Init(cliConfig, namespace, "secret", klog.V(4).Infof); err != nil {
		return nil, err
	}
	return actionConfig, nil
}

func HelmInit(ctx context.Context, namespace string, kubeconfig string) (*helmCli.EnvSettings, *helmAction.Configuration, error) {
	//log := ctrl.LoggerFrom(ctx)

	settings := helmCli.New()

	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, nil, err
	}

	actionConfig, err := GetActionConfig(ctx, namespace, restConfig)
	if err != nil {
		return nil, nil, err
	}

	return settings, actionConfig, nil
}

func InstallOrUpgradeHelmReleases(ctx context.Context, kubeconfig, ocneCPName string, spec bootstrapv1.ModuleAddons) (*helmRelease.Release, error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Installing or upgrading Helm release")
	existingRelease, err := GetHelmRelease(ctx, kubeconfig, spec)
	if err != nil {
		if err == helmDriver.ErrReleaseNotFound {
			return InstallHelmRelease(ctx, kubeconfig, ocneCPName, spec)
		}

		return nil, err
	}

	return UpgradeHelmReleaseIfChanged(ctx, kubeconfig, spec, existingRelease)
}

func InstallHelmRelease(ctx context.Context, kubeconfig, ocneCPName string, spec bootstrapv1.ModuleAddons) (*helmRelease.Release, error) {
	log := ctrl.LoggerFrom(ctx)

	settings, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}
	installClient := helmAction.NewInstall(actionConfig)
	installClient.RepoURL = spec.RepoURL
	installClient.Version = spec.Version
	installClient.Namespace = spec.ReleaseNamespace
	installClient.CreateNamespace = true

	if spec.ReleaseName == "" {
		installClient.GenerateName = true
		spec.ReleaseName, _, err = installClient.NameAndChart([]string{spec.ReleaseName})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate release name for chart %s on ocne control plane %s", spec.ReleaseName, ocneCPName)
		}
	}
	installClient.ReleaseName = spec.ReleaseName

	log.V(2).Info("Locating chart...")
	cp, err := installClient.ChartPathOptions.LocateChart(spec.ReleaseName, settings)
	if err != nil {
		return nil, err
	}
	log.V(2).Info("Located chart at path", "path", cp)

	log.V(2).Info("Writing values to file")
	filename, err := writeValuesToFile(ctx, spec)
	if err != nil {
		return nil, err
	}
	defer os.Remove(filename)
	log.V(2).Info("Values written to file", "path", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("Values written to file %s are:\n%s\n", filename, string(content))

	p := helmGetter.All(settings)
	valueOpts := &helmVals.Options{
		ValueFiles: []string{filename},
	}
	vals, err := valueOpts.MergeValues(p)
	if err != nil {
		return nil, err
	}
	chartRequested, err := helmLoader.Load(cp)

	if err != nil {
		return nil, err
	}
	log.V(2).Info("Installing with Helm...")

	return installClient.RunWithContext(ctx, chartRequested, vals) // Can return error and a release
}

func UpgradeHelmReleaseIfChanged(ctx context.Context, kubeconfig string, spec bootstrapv1.ModuleAddons, existing *helmRelease.Release) (*helmRelease.Release, error) {
	log := ctrl.LoggerFrom(ctx)

	settings, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}
	upgradeClient := helmAction.NewUpgrade(actionConfig)
	upgradeClient.RepoURL = spec.RepoURL
	upgradeClient.Version = spec.Version
	upgradeClient.Namespace = spec.ReleaseNamespace
	log.V(2).Info("Locating chart...")
	cp, err := upgradeClient.ChartPathOptions.LocateChart(spec.ReleaseName, settings)
	if err != nil {
		return nil, err
	}
	log.V(2).Info("Located chart at path", "path", cp)

	log.V(2).Info("Writing values to file")
	filename, err := writeValuesToFile(ctx, spec)
	if err != nil {
		return nil, err
	}
	defer os.Remove(filename)
	log.V(2).Info("Values written to file", "path", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("Values written to file %s are:\n%s\n", filename, string(content))

	p := helmGetter.All(settings)
	valueOpts := &helmVals.Options{
		ValueFiles: []string{filename},
	}
	vals, err := valueOpts.MergeValues(p)
	if err != nil {
		return nil, err
	}
	chartRequested, err := helmLoader.Load(cp)
	if err != nil {
		return nil, err
	}
	if chartRequested == nil {
		return nil, errors.Errorf("failed to load request chart %s", spec.ReleaseName)
	}

	shouldUpgrade, err := shouldUpgradeHelmRelease(ctx, *existing, chartRequested, vals)
	if err != nil {
		return nil, err
	}
	if !shouldUpgrade {
		log.V(2).Info(fmt.Sprintf("Release `%s` is up to date, no upgrade requried, revision = %d", existing.Name, existing.Version))
		return existing, nil
	}

	log.V(2).Info(fmt.Sprintf("Upgrading release `%s` with Helm", spec.ReleaseName))
	// upgrader.DryRun = true
	release, err := upgradeClient.RunWithContext(ctx, spec.ReleaseName, chartRequested, vals)

	return release, err
	// Should we force upgrade if it failed previously?
}

func writeValuesToFile(ctx context.Context, spec bootstrapv1.ModuleAddons) (string, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Writing values to file")
	valuesFile, err := os.CreateTemp("", spec.ReleaseName+"-"+spec.ReleaseName+"-*.yaml")
	if err != nil {
		return "", err
	}

	if _, err := valuesFile.Write([]byte(spec.Values)); err != nil {
		return "", err
	}

	return valuesFile.Name(), nil
}

func shouldUpgradeHelmRelease(ctx context.Context, existing helmRelease.Release, chartRequested *chart.Chart, values map[string]interface{}) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	if existing.Chart == nil || existing.Chart.Metadata == nil {
		return false, errors.New("Failed to resolve chart version of existing release")
	}
	if existing.Chart.Metadata.Version != chartRequested.Metadata.Version {
		log.V(3).Info("Versions are different, upgrading")
		return true, nil
	}

	if existing.Info.Status == helmRelease.StatusFailed {
		log.Info("Release is in failed state, attempting upgrade to fix it")
		return true, nil
	}

	klog.V(2).Infof("Diff between values is:\n%s", cmp.Diff(existing.Config, values))

	// TODO: Comparing yaml is not ideal, but it's the best we can do since DeepEquals fails. This is because int64 types
	// are converted to float64 when returned from the helm API.
	oldValues, err := yaml.Marshal(existing.Config)
	if err != nil {
		return false, errors.Wrapf(err, "failed to marshal existing release values")
	}
	newValues, err := yaml.Marshal(values)
	if err != nil {
		return false, errors.Wrapf(err, "failed to new release values")
	}

	return !cmp.Equal(oldValues, newValues), nil
}

func GetHelmRelease(ctx context.Context, kubeconfig string, spec bootstrapv1.ModuleAddons) (*helmRelease.Release, error) {
	if spec.ReleaseName == "" {
		return nil, helmDriver.ErrReleaseNotFound
	}

	_, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}
	getClient := helmAction.NewGet(actionConfig)
	release, err := getClient.Run(spec.ReleaseName)
	if err != nil {
		return nil, err
	}

	return release, nil
}

func UninstallHelmRelease(ctx context.Context, kubeconfig string, spec bootstrapv1.ModuleAddons) (*helmRelease.UninstallReleaseResponse, error) {
	_, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}

	uninstallClient := helmAction.NewUninstall(actionConfig)
	response, err := uninstallClient.Run(spec.ReleaseName)
	if err != nil {
		return nil, err
	}

	return response, nil
}
