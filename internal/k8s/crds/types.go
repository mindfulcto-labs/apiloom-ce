// Package crds defines Kubernetes Custom Resource Definition types for FlexGateway.
// Group: flexgateway.io, Version: v1alpha1.
//
// These types are hand-written (no controller-gen or kubebuilder dependency).
// CRD YAML manifests are maintained alongside these types.
package crds

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FlexRoute defines a FlexGateway route as a Kubernetes custom resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type FlexRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec FlexRouteSpec `json:"spec"`
}

// FlexRouteSpec defines the desired state of a FlexRoute.
type FlexRouteSpec struct {
	Name     string            `json:"name"`
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Backend  string            `json:"backend"`
	Blocks   []BlockConfig     `json:"blocks,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// BlockConfig references a block (middleware) and its configuration.
type BlockConfig struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config,omitempty"`
}

// FlexRouteList contains a list of FlexRoute resources.
// +kubebuilder:object:root=true
type FlexRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []FlexRoute `json:"items"`
}

// FlexConsumer defines a FlexGateway API consumer as a Kubernetes custom resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type FlexConsumer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec FlexConsumerSpec `json:"spec"`
}

// FlexConsumerSpec defines the desired state of a FlexConsumer.
type FlexConsumerSpec struct {
	Name      string            `json:"name"`
	Org       string            `json:"org"`
	APIKeys   []string          `json:"apiKeys,omitempty"`
	RateLimit *int              `json:"rateLimit,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// FlexConsumerList contains a list of FlexConsumer resources.
// +kubebuilder:object:root=true
type FlexConsumerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []FlexConsumer `json:"items"`
}

// FlexPlugin defines a FlexGateway plugin as a Kubernetes custom resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type FlexPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec FlexPluginSpec `json:"spec"`
}

// FlexPluginSpec defines the desired state of a FlexPlugin.
type FlexPluginSpec struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Config  json.RawMessage `json:"config,omitempty"`
	Targets []TargetRef     `json:"targets,omitempty"`
	Enabled bool            `json:"enabled"`
}

// TargetRef references a route or consumer that this plugin targets.
type TargetRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// FlexPluginList contains a list of FlexPlugin resources.
// +kubebuilder:object:root=true
type FlexPluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []FlexPlugin `json:"items"`
}
