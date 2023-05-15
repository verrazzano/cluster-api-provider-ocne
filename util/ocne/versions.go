// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocne

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"os"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"
)

type OCNEMetadata struct {
	OCNEImages `json:"container-images"`
	Release    string `json:release,omitempty`
}

type OCNEImages struct {
	ETCD           string `json:"etcd"`
	CoreDNS        string `json:"coredns"`
	TigeraOperator string `json:"tigera-operator"`
	Calico         string `json:"calico"`
}

const (
	defaultTigeraOperatorTag = "v1.29.0"
	defaultCalicoTag         = "v3.25.0"
	minOCNEVersion           = "v1.24.8"
	configMapName            = "ocne-metadata"
	cmDataKey                = "mapping"
)

var k8s_ocne_version_maping = map[string]string{
	"v1.25.7":  "1.6",
	"v1.24.8":  "1.5",
	"v1.24.5":  "1.5",
	"v1.23.14": "1.5",
	"v1.23.11": "1.5",
	"v1.23.7":  "1.5",
	"v1.22.16": "1.5",
	"v1.22.14": "1.5",
	"v1.22.8":  "1.5",
}

var getCoreV1Func = getCoreV1Client

func getCoreV1Client() (v1.CoreV1Interface, error) {
	restConfig := controllerruntime.GetConfigOrDie()

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return kubeClient.CoreV1(), nil
}

func CreateOCNEMetadataConfigMap(ctx context.Context, metadataFile string) error {
	// Only create OCNE Metadata if file is present
	if _, err := os.Stat(metadataFile); err != nil {
		return nil
	}

	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return err
	}

	rawMapping := map[string]OCNEMetadata{}
	if err := yaml.Unmarshal(data, &rawMapping); err != nil {
		return err
	}

	mapping, err := buildMapping(rawMapping)
	if err != nil {
		return nil
	}

	mappingBytes, err := yaml.Marshal(mapping)
	if err != nil {
		return err
	}

	cmData := map[string]string{
		cmDataKey: string(mappingBytes),
	}
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = "default"
	}
	client, err := getCoreV1Func()
	if err != nil {
		return err
	}

	cm, getErr := client.ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if getErr != nil {
		// create new configmap if not found
		if apierrors.IsNotFound(getErr) {
			_, createErr := client.ConfigMaps(namespace).Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
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

func buildMapping(rawMapping map[string]OCNEMetadata) (map[string]OCNEMetadata, error) {
	minSupportedVersion, err := NewSemVersion(minOCNEVersion)
	if err != nil {
		return nil, err
	}
	result := map[string]OCNEMetadata{}
	for version, meta := range rawMapping {
		if ok, _ := isSupported(version, minSupportedVersion); ok {
			if version[0] != 'v' {
				version = fmt.Sprintf("v%s", version)
			}

			// Add OCNE Defaults if metadata not present in yum
			if meta.OCNEImages.TigeraOperator == "" {
				meta.OCNEImages.TigeraOperator = defaultTigeraOperatorTag
			}
			if meta.OCNEImages.Calico == "" {
				meta.OCNEImages.Calico = defaultCalicoTag
			}
			// Add release if missing
			if meta.Release == "" {
				ocneVersion, ok := k8s_ocne_version_maping[version]
				if ok {
					meta.Release = ocneVersion
				}
			}
			result[version] = meta
		}
	}
	return result, nil
}

func isSupported(version string, minVersion *SemVersion) (bool, error) {
	v, err := NewSemVersion(version)
	if err != nil {
		return false, err
	}

	return v.IsGreaterThanOrEqualTo(minVersion), nil
}
