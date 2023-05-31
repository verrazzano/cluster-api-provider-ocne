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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"text/template"
)

const (
	ocneModuleOperatorRepo      = "ghcr.io"
	ocneModuleOperatorNamespace = "verrazzano"
	ocneModuleOperatorChartName = "verrazzano-module-operator"
	OCNEModuleOperatorImageName = "module-operator"
	defaultImagePullPolicy      = "IfNotPresent"
	ocneModuleOperatorPath      = "charts/operators/verrazzano-module-operator/"
)

var (
	//go:embed imageMeta.tmpl
	valuesTemplate string

	defaultTemplateFuncMap = template.FuncMap{
		"Indent": templateYAMLIndent,
	}
)

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)
	return strings.Repeat(" ", i) + strings.Join(split, ident)
}

func generate(kind string, tpl string, data interface{}) ([]byte, error) {
	spew.Dump(data)
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}
	spew.Dump(string(out.Bytes()))
	return out.Bytes(), nil
}

func getDefaultOCNEModuleOperatorImageRepo() string {
	return fmt.Sprintf("%s/%s/%s", ocneModuleOperatorRepo, ocneModuleOperatorNamespace, OCNEModuleOperatorImageName)
}

func generateDataValues(ctx context.Context, spec *controlplanev1.ModuleOperator, k8sVersion string) ([]byte, error) {
	log := ctrl.LoggerFrom(ctx)
	ocneMeta, err := ocne.GetOCNEMetadata(context.Background())
	if err != nil {
		log.Error(err, "failed to get retrieve OCNE metadata")
		return nil, err

	}

	var helmMeta HelmValuesTemplate

	// Setting default values for image
	if spec.Image != nil {
		// Set defaults or honour overrides
		if spec.Image.Repository == "" {
			helmMeta.Repository = getDefaultOCNEModuleOperatorImageRepo()
		} else {
			imageList := strings.Split(strings.Trim(strings.TrimSpace(spec.Image.Repository), "/"), "/")
			if imageList[len(imageList)-1] == OCNEModuleOperatorImageName {
				helmMeta.Repository = spec.Image.Repository
			} else {
				helmMeta.Repository = fmt.Sprintf("%s/%s", strings.Join(imageList[0:len(imageList)], "/"), OCNEModuleOperatorImageName)
			}
		}

		if spec.Image.Tag == "" {
			helmMeta.Tag = ocneMeta[k8sVersion].OCNEImages.ModuleOperator
		} else {
			helmMeta.Tag = strings.TrimSpace(spec.Image.Tag)
		}

		if spec.Image.PullPolicy == "" {
			helmMeta.PullPolicy = defaultImagePullPolicy
		} else {
			helmMeta.PullPolicy = strings.TrimSpace(spec.Image.PullPolicy)
		}

		if spec.ImagePullSecrets != nil {
			helmMeta.ImagePullSecrets = spec.ImagePullSecrets
		}

	} else {
		// If nothing has been specified in API
		helmMeta = HelmValuesTemplate{
			Repository:       getDefaultOCNEModuleOperatorImageRepo(),
			Tag:              ocneMeta[k8sVersion].OCNEImages.ModuleOperator,
			PullPolicy:       defaultImagePullPolicy,
			ImagePullSecrets: []controlplanev1.SecretName{},
		}
	}

	return generate("HelmValues", valuesTemplate, helmMeta)
}

func GetOCNEModuleOperatorAddons(ctx context.Context, spec *controlplanev1.ModuleOperator, k8sVersion string) (*HelmModuleAddons, error) {
	log := ctrl.LoggerFrom(ctx)
	out, err := generateDataValues(ctx, spec, k8sVersion)
	if err != nil {
		log.Error(err, "failed to generate data")
		return nil, err
	}

	return &HelmModuleAddons{
		ChartName:        ocneModuleOperatorChartName,
		ReleaseName:      ocneModuleOperatorChartName,
		ReleaseNamespace: ocneModuleOperatorChartName,
		RepoURL:          ocneModuleOperatorPath,
		Local:            true,
		ValuesTemplate:   string(out),
	}, nil

}
