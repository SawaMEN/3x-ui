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
	byTag := make(map[string]map[string]any, len(generated))
	for _, item := range generated {
		tag, _ := item["tag"].(string)
		byTag[tag] = item
	}
	seen := make(map[string]bool, len(generated))
	var result []map[string]any
	for _, item := range template {
		tag, _ := item["tag"].(string)
		if !tags[tag] {
			result = append(result, item)
		} else if current, ok := byTag[tag]; ok && !seen[tag] {
			// Replace stale editor copies in place: the first outbound is the
			// implicit default when route.final is empty.
			result = append(result, current)
			seen[tag] = true
		}
	}
	for _, item := range generated {
		tag, _ := item["tag"].(string)
		if !seen[tag] {
			result = append(result, item)
			seen[tag] = true
		}
	}
	return result
}
