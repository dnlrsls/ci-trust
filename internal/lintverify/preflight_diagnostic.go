package lintverify

// Only closed workflow stages may enter reports, never underlying errors.
// Delivery and its following review check deliberately share an honest stage.
func preflightDetail(stage string) string {
	switch stage {
	case "REVIEW_BEFORE_DELIVERY", "ARTIFACT_DELIVERY_OR_REVIEW", "RUNTIME_CREATION", "IMAGE_LOAD":
		return " Preflight detail: stage=" + stage + ". This metadata does not establish Node execution."
	}
	return ""
}
