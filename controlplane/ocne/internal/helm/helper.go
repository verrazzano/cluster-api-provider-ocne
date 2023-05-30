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
	ocneModuleOperatorImageName = "verrazzano-module-operator"
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
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	return out.Bytes(), nil
}

func getImageRepo() string {
	return fmt.Sprintf("%s/%s/%s", ocneModuleOperatorRepo, ocneModuleOperatorNamespace, ocneModuleOperatorImageName)
}

func generateDataValues(ctx context.Context, spec *controlplanev1.ModuleOperator, k8sVersion string) ([]byte, error) {
	log := ctrl.LoggerFrom(ctx)
	ocneMeta, err := ocne.GetOCNEMetadata(context.Background())
	if err != nil {
		log.Error(err, "failed to get retrieve OCNE metadata")
		return nil, err

	}

	// Setting default values for image
	if spec.Image.Repository == "" {
		spec.Image.Repository = getImageRepo()
	}

	if spec.Image.Tag == "" {
		spec.Image.Tag = ocneMeta[k8sVersion].OCNEImages.OCNEModuleOperator
	}

	if spec.Image.PullPolicy == "" {
		spec.Image.PullPolicy = defaultImagePullPolicy
	}

	return generate("HelmValues", valuesTemplate, spec.Image)
}

func GetOCNEModuleOperatorAddons(ctx context.Context, spec *controlplanev1.ModuleOperator, k8sVersion string) (*HelmModuleAddons, error) {
	log := ctrl.LoggerFrom(ctx)
	out, err := generateDataValues(ctx, spec, k8sVersion)
	if err != nil {
		log.Error(err, "failed to generate data")
		return nil, err
	}

	return &HelmModuleAddons{
		ChartName:        ocneModuleOperatorImageName,
		ReleaseName:      ocneModuleOperatorImageName,
		ReleaseNamespace: ocneModuleOperatorImageName,
		RepoURL:          ocneModuleOperatorPath,
		Local:            true,
		ValuesTemplate:   string(out),
	}, nil

}
