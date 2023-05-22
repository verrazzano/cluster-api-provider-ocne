// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocne

import (
	"context"
	"helm.sh/helm/v3/pkg/repo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"sigs.k8s.io/yaml"
)

const (
	moduleConfigMapName = "module-metadata"
	cmModuleDataKey     = "entries"
)

type ModuleMetadata struct {
	Entries map[string]repo.ChartVersions
}

func CreateModuleMetadataConfigMap(ctx context.Context, metadataFile string) error {
	// Only create module metadata if file is present
	if _, err := os.Stat(metadataFile); err != nil {
		return nil
	}

	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return err
	}

	indexFile := repo.IndexFile{}
	if err := yaml.Unmarshal(data, &indexFile); err != nil {
		return err
	}

	charts := ModuleMetadata{Entries: indexFile.Entries}

	chartBytes, err := yaml.Marshal(charts.Entries)
	if err != nil {
		return err
	}

	cmData := map[string]string{
		cmModuleDataKey: string(chartBytes),
	}
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = "default"
	}
	client, err := getCoreV1Func()
	if err != nil {
		return err
	}

	cm, getErr := client.ConfigMaps(namespace).Get(ctx, moduleConfigMapName, metav1.GetOptions{})
	if getErr != nil {
		// create new configmap if not found
		if apierrors.IsNotFound(getErr) {
			_, createErr := client.ConfigMaps(namespace).Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleConfigMapName,
					Namespace: namespace,
				},
				Data: cmData,
			}, metav1.CreateOptions{})
			return createErr
		}
		return getErr
	}
	// update configmap if it already exists
	cm.Data = cmData
	if _, updateErr := client.ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{}); updateErr != nil {
		return updateErr
	}
	return nil
}
