// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocne

import (
	"context"
	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corev1Cli "k8s.io/client-go/kubernetes/typed/core/v1"
	"os"
	"sigs.k8s.io/yaml"
	"testing"
)

const testFileName = "testdata/kubernetes_versions.yaml"

func TestLoadMetadata(t *testing.T) {
	getCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset().CoreV1(), nil
	}
	defer func() { getCoreV1Func = getCoreV1Client }()
	err := CreateOCNEMetadataConfigMap(context.TODO(), testFileName)
	assert.NoError(t, err)
}

func TestMetadataContent(t *testing.T) {
	data, err := os.ReadFile(testFileName)
	assert.NoError(t, err)

	rawMapping := map[string]OCNEMetadata{}
	err = yaml.Unmarshal(data, &rawMapping)
	assert.NoError(t, err)

	mapping, err := buildMapping(rawMapping)
	assert.NoError(t, err)
	assert.Equal(t, len(mapping), 1)

	assert.Empty(t, mapping["v1.24.8"])
	assert.NotEmpty(t, mapping["v1.25.7"])

	assert.Equal(t, mapping["v1.25.7"].Release, "1.6")
}
