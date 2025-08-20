package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

// extractUsernameFromRequest extracts the username from the ServiceAccount token
func extractUsernameFromRequest(req admission.Request) string {
	// Handle empty username
	if req.UserInfo.Username == "" {
		return ""
	}

	// Check if the username follows the ServiceAccount format: system:serviceaccount:<namespace>:<serviceaccount-name>
	const serviceAccountPrefix = "system:serviceaccount:"
	if strings.HasPrefix(req.UserInfo.Username, serviceAccountPrefix) {
		parts := strings.Split(req.UserInfo.Username, ":")
		// Ensure the username has exactly 4 parts and is a valid ServiceAccount format
		if len(parts) == 4 && parts[0] == "system" && parts[1] == "serviceaccount" && parts[3] != "" {
			return parts[3]
		}
	}

	// For non-ServiceAccount authentication, return "manual"
	return "manual"
}
