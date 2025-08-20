package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	annotations2 "github.com/mayooot/serviceaccount-quota/pkg/annotations"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const skipInjectKey = "mayooot.github.io/skip-inject-username"

type mutating struct {
	decoder admission.Decoder
}

func NewMutating() *admission.Webhook {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(fmt.Errorf("failed to add client to scheme: %v", err))
	}
	decoder := admission.NewDecoder(scheme)
	wh := &admission.Webhook{Handler: &mutating{decoder: decoder}}
	return wh
}

// Handle processes the admission request and injects the service account username annotation.
func (s *mutating) Handle(_ context.Context, req admission.Request) admission.Response {
	// Build log fields
	logFields := []interface{}{
		"resource", req.Resource.Resource,
		"namespace", req.Namespace,
		"name", req.Name,
		"requestUID", req.UID,
	}

	// Extract service account name from request
	username := extractUsernameFromRequest(req)
	if username == "" {
		klog.Warning("Could not extract username from request, skipping annotation", logFields)
		return admission.Allowed("No username found in request")
	}

	// Decode the object as unstructured
	obj := &unstructured.Unstructured{}
	if err := s.decoder.Decode(req, obj); err != nil {
		klog.ErrorS(err, "Failed to decode object as unstructured", logFields...)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode object: %v", err))
	}

	// Get or initialize annotations
	annotations, _, err := unstructured.NestedMap(obj.Object, "metadata", "annotations")
	if err != nil {
		annotations = make(map[string]interface{})
	}
	if annotations == nil {
		annotations = make(map[string]interface{})
	}

	// Check if injection should be skipped
	if val, ok := annotations[skipInjectKey]; ok && val == "true" {
		klog.InfoS("Skipping username injection", logFields...)
		return admission.Allowed("Skipping username injection due to annotation")
	}

	// Inject username annotation
	klog.InfoS("Injecting username annotation", append(logFields, "username", username)...)
	annotations[annotations2.ServiceAccountKey] = username

	// Create JSON patch
	patch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": annotations,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		klog.ErrorS(err, "Failed to marshal patch", logFields...)
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patch: %v", err))
	}

	// Return patched response
	return admission.PatchResponseFromRaw(req.Object.Raw, patchBytes)
}
