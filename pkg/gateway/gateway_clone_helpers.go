package gateway

func cloneBindingConfig(items map[string]string) map[string]string {
	if len(items) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyValues(items map[string]any) map[string]any {
	if len(items) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}
