package crds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// testdataDir returns the directory containing CRD YAML files.
func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

func TestFlexRouteJSONRoundTrip(t *testing.T) {
	route := FlexRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flexgateway.io/v1alpha1",
			Kind:       "FlexRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "default",
		},
		Spec: FlexRouteSpec{
			Name:     "test-route",
			Endpoint: "/api/v1/test",
			Method:   "GET",
			Backend:  "test-backend",
			Blocks: []BlockConfig{
				{Name: "rate-limit", Config: json.RawMessage(`{"rpm":100}`)},
			},
			Metadata: map[string]string{"env": "test"},
		},
	}

	data, err := json.Marshal(route)
	require.NoError(t, err)

	var decoded FlexRoute
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, "flexgateway.io/v1alpha1", decoded.APIVersion)
	require.Equal(t, "FlexRoute", decoded.Kind)
	require.Equal(t, "test-route", decoded.Spec.Name)
	require.Equal(t, "/api/v1/test", decoded.Spec.Endpoint)
	require.Equal(t, "GET", decoded.Spec.Method)
	require.Equal(t, "test-backend", decoded.Spec.Backend)
	require.Len(t, decoded.Spec.Blocks, 1)
	require.Equal(t, "rate-limit", decoded.Spec.Blocks[0].Name)
	require.Equal(t, "test", decoded.Spec.Metadata["env"])
}

func TestFlexConsumerJSONRoundTrip(t *testing.T) {
	consumer := FlexConsumer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flexgateway.io/v1alpha1",
			Kind:       "FlexConsumer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-consumer",
			Namespace: "default",
		},
		Spec: FlexConsumerSpec{
			Name:      "test-consumer",
			Org:       "acme",
			APIKeys:   []string{"key-abc", "key-def"},
			RateLimit: intPtr(1000),
			Metadata:  map[string]string{"tier": "premium"},
		},
	}

	data, err := json.Marshal(consumer)
	require.NoError(t, err)

	var decoded FlexConsumer
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, "flexgateway.io/v1alpha1", decoded.APIVersion)
	require.Equal(t, "FlexConsumer", decoded.Kind)
	require.Equal(t, "test-consumer", decoded.Spec.Name)
	require.Equal(t, "acme", decoded.Spec.Org)
	require.Equal(t, []string{"key-abc", "key-def"}, decoded.Spec.APIKeys)
	require.Equal(t, 1000, *decoded.Spec.RateLimit)
	require.Equal(t, "premium", decoded.Spec.Metadata["tier"])
}

func TestFlexPluginJSONRoundTrip(t *testing.T) {
	plugin := FlexPlugin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flexgateway.io/v1alpha1",
			Kind:       "FlexPlugin",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-plugin",
			Namespace: "default",
		},
		Spec: FlexPluginSpec{
			Name:    "test-plugin",
			Type:    "native",
			Config:  json.RawMessage(`{"key":"value"}`),
			Targets: []TargetRef{{Kind: "FlexRoute", Name: "my-route"}},
			Enabled: true,
		},
	}

	data, err := json.Marshal(plugin)
	require.NoError(t, err)

	var decoded FlexPlugin
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, "flexgateway.io/v1alpha1", decoded.APIVersion)
	require.Equal(t, "FlexPlugin", decoded.Kind)
	require.Equal(t, "test-plugin", decoded.Spec.Name)
	require.Equal(t, "native", decoded.Spec.Type)
	require.True(t, decoded.Spec.Enabled)
	require.Len(t, decoded.Spec.Targets, 1)
	require.Equal(t, "FlexRoute", decoded.Spec.Targets[0].Kind)
}

func TestCRDYAMLParsesFlexRoute(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(), "flexroute.yaml"))
	require.NoError(t, err)

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(data, &crd)
	require.NoError(t, err)

	require.Equal(t, "CustomResourceDefinition", crd.Kind)
	require.Equal(t, "flexgateway.io", crd.Spec.Group)
	require.Equal(t, "FlexRoute", crd.Spec.Names.Kind)
	require.Equal(t, "flexroutes", crd.Spec.Names.Plural)
	require.Equal(t, apiextensionsv1.NamespaceScoped, crd.Spec.Scope)
	require.Len(t, crd.Spec.Versions, 1)
	require.Equal(t, "v1alpha1", crd.Spec.Versions[0].Name)
	require.True(t, crd.Spec.Versions[0].Served)
	require.True(t, crd.Spec.Versions[0].Storage)
}

func TestCRDYAMLParsesFlexConsumer(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(), "flexconsumer.yaml"))
	require.NoError(t, err)

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(data, &crd)
	require.NoError(t, err)

	require.Equal(t, "CustomResourceDefinition", crd.Kind)
	require.Equal(t, "flexgateway.io", crd.Spec.Group)
	require.Equal(t, "FlexConsumer", crd.Spec.Names.Kind)
	require.Equal(t, "flexconsumers", crd.Spec.Names.Plural)
	require.Equal(t, apiextensionsv1.NamespaceScoped, crd.Spec.Scope)
}

func TestCRDYAMLParsesFlexPlugin(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(), "flexplugin.yaml"))
	require.NoError(t, err)

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(data, &crd)
	require.NoError(t, err)

	require.Equal(t, "CustomResourceDefinition", crd.Kind)
	require.Equal(t, "flexgateway.io", crd.Spec.Group)
	require.Equal(t, "FlexPlugin", crd.Spec.Names.Kind)
	require.Equal(t, "flexplugins", crd.Spec.Names.Plural)
	require.Equal(t, apiextensionsv1.NamespaceScoped, crd.Spec.Scope)
}

func intPtr(v int) *int {
	return &v
}
