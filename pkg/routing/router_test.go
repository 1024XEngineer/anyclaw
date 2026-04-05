package routing

import "testing"

func TestRoutingRuleMatchesRegexPattern(t *testing.T) {
	rule := RoutingRule{Pattern: "打开.*应用"}
	if !rule.Matches("帮我打开 Steam 应用") {
		t.Fatalf("expected regex-style Chinese pattern to match open-app request")
	}
}

func TestClassifyTaskHandlesChineseKeywords(t *testing.T) {
	router := &Router{}
	got := router.classifyTask("请帮我搜索这个网站")
	if got != CategoryWebBrowser {
		t.Fatalf("expected web browser category, got %s", got.String())
	}
}
