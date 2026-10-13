package service

func selectSubscriptionConnections(items []map[string]any, tags map[string]bool) []map[string]any {
	var selected []map[string]any
	for _, item := range items {
		if tag, _ := item["tag"].(string); tags[tag] {
			selected = append(selected, item)
		}
	}
	return selected
}

// Native templates own manual connections. Managed profiles always come from
// the current subscription rather than stale copies saved in the editor.
func mergeSubscriptionConnections(template, generated []map[string]any, tags map[string]bool) []map[string]any {
	var result []map[string]any
	for _, item := range template {
		if tag, _ := item["tag"].(string); !tags[tag] {
			result = append(result, item)
		}
	}
	return append(result, generated...)
}
