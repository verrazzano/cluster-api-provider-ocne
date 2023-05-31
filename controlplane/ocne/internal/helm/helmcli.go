/*
Copyright 2022 The Kubernetes Authors.

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
	if err := actionConfig.Init(cliConfig, namespace, "secret", klog.V(4).Infof); err != nil {
		return nil, err
	}
	return actionConfig, nil
}

func HelmInit(ctx context.Context, namespace string, kubeconfig string) (*helmCli.EnvSettings, *helmAction.Configuration, error) {
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

func InstallOrUpgradeHelmReleases(ctx context.Context, kubeconfig, ocneCPName, values string, spec *HelmModuleAddons) (*helmRelease.Release, error) {
	klog.V(2).Info("Installing or upgrading Helm release")
	existingRelease, err := GetHelmRelease(ctx, kubeconfig, spec)
	if err != nil {
		if err == helmDriver.ErrReleaseNotFound {
			return InstallHelmRelease(ctx, kubeconfig, ocneCPName, values, spec)
		}
		return nil, err
	}

	return UpgradeHelmReleaseIfChanged(ctx, kubeconfig, values, spec, existingRelease)
}

func InstallHelmRelease(ctx context.Context, kubeconfig, ocneCPName, values string, spec *HelmModuleAddons) (*helmRelease.Release, error) {
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
		spec.ReleaseName, _, err = installClient.NameAndChart([]string{spec.ChartName})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate release name for chart %s on ocne control plane %s", spec.ChartName, ocneCPName)
		}
	}
	installClient.ReleaseName = spec.ReleaseName

	var cp string
	if spec.Local {
		cp = spec.RepoURL
	} else {
		klog.V(2).Info("Locating chart...")
		cp, err = installClient.ChartPathOptions.LocateChart(spec.ChartName, settings)
		if err != nil {
			log.Error(err, "Unable to find chart")
			return nil, err
		}
	}

	klog.V(2).Info(fmt.Sprintf("Located chart at path '%s'", cp))
	klog.V(2).Info("Writing values to file")

	filename, err := writeValuesToFile(ctx, values, spec)
	if err != nil {
		return nil, err
	}
	defer os.Remove(filename)
	klog.V(2).Info("Values written to file", "path", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	klog.V(2).Info(fmt.Sprintf("Values written to file %s are:\n%s\n", filename, string(content)))

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
	klog.V(2).Info("Installing with Helm...")

	return installClient.RunWithContext(ctx, chartRequested, vals)
}

func UpgradeHelmReleaseIfChanged(ctx context.Context, kubeconfig, values string, spec *HelmModuleAddons, existing *helmRelease.Release) (*helmRelease.Release, error) {
	log := ctrl.LoggerFrom(ctx)

	settings, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}
	upgradeClient := helmAction.NewUpgrade(actionConfig)
	upgradeClient.RepoURL = spec.RepoURL
	upgradeClient.Version = spec.Version
	upgradeClient.Namespace = spec.ReleaseNamespace

	var cp string
	if spec.Local {
		klog.V(2).Info("Local path...")
		cp = spec.RepoURL
	} else {
		klog.V(2).Info("Locating chart...")
		cp, err = upgradeClient.ChartPathOptions.LocateChart(spec.ChartName, settings)
		if err != nil {
			klog.Error(err, "Unable to find chart")
			return nil, err
		}
	}

	klog.V(2).Info(fmt.Sprintf("Located chart at path '%s'", cp))
	klog.V(2).Info("Writing values to file")
	filename, err := writeValuesToFile(ctx, values, spec)
	if err != nil {
		return nil, err
	}
	defer os.Remove(filename)
	klog.V(2).Info("Values written to file", "path", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	klog.V(2).Info(fmt.Sprintf("Values written to file %s are:\n%s\n", filename, string(content)))

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
		return nil, errors.Errorf("failed to load request chart %s", spec.ChartName)
	}

	shouldUpgrade, err := shouldUpgradeHelmRelease(ctx, *existing, chartRequested, vals)
	if err != nil {
		return nil, err
	}
	if !shouldUpgrade {
		log.Info(fmt.Sprintf("Release `%s` is up to date, no upgrade requried, revision = %d", existing.Name, existing.Version))
		return existing, nil
	}

	log.Info(fmt.Sprintf("Upgrading release `%s` with Helm", spec.ReleaseName))
	//upgradeClient.DryRun = true
	return upgradeClient.RunWithContext(ctx, spec.ReleaseName, chartRequested, vals)
}

func writeValuesToFile(ctx context.Context, values string, spec *HelmModuleAddons) (string, error) {
	klog.V(2).Info("Writing values to file")
	valuesFile, err := os.CreateTemp("", spec.ChartName+"-"+spec.ReleaseName+"-*.yaml")
	if err != nil {
		return "", err
	}

	if _, err := valuesFile.Write([]byte(values)); err != nil {
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

func GetHelmRelease(ctx context.Context, kubeconfig string, spec *HelmModuleAddons) (*helmRelease.Release, error) {
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

func UninstallHelmRelease(ctx context.Context, kubeconfig string, spec *HelmModuleAddons) (*helmRelease.UninstallReleaseResponse, error) {
	log := ctrl.LoggerFrom(ctx)
	_, actionConfig, err := HelmInit(ctx, spec.ReleaseNamespace, kubeconfig)
	if err != nil {
		return nil, err
	}
	log.Info(fmt.Sprintf("Uninstalling helm chart '%s' ...", spec.ReleaseName))
	uninstallClient := helmAction.NewUninstall(actionConfig)
	response, err := uninstallClient.Run(spec.ReleaseName)
	if err != nil {
		return nil, err
	}

	return response, nil
}
