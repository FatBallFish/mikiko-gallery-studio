package mgsctl

import "testing"

func TestI18NMessageCatalogsAreComplete(t *testing.T) {
	for key := range tuiMessages[LanguageChinese] {
		if tuiMessages[LanguageEnglish][key] == "" {
			t.Errorf("English catalog is missing %q", key)
		}
	}
	for key := range tuiMessages[LanguageEnglish] {
		if tuiMessages[LanguageChinese][key] == "" {
			t.Errorf("Chinese catalog is missing %q", key)
		}
	}
}

func TestI18NFallsBackToChinese(t *testing.T) {
	if got := tuiMessage("unsupported", "root.install"); got != "安装与部署" {
		t.Fatalf("fallback message = %q", got)
	}
}
