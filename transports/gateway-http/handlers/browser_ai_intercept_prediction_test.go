package handlers

import (
	"testing"

	"github.com/gateway/gateway/framework/logstore"
)

func TestAIBotRiskPredictionMatchesAction(t *testing.T) {
	cases := []struct {
		name      string
		action    string
		score     int
		severity  string
		wantScore int
		wantRisk  string
	}{
		{"low severity block is still high risk", "BLOCK", 20, "LOW", 90, "HIGH"},
		{"critical redact remains high risk", "REDACT", 95, "CRITICAL", 60, "HIGH"},
		{"medium redact remains medium risk", "REDACT", 50, "MEDIUM", 50, "MEDIUM"},
		{"critical warning is medium risk", "WARN", 95, "CRITICAL", 40, "MEDIUM"},
		{"low warning stays low risk", "WARN", 10, "LOW", 25, "LOW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotScore, gotRisk := aiBotRiskPrediction(tc.action, tc.score, tc.severity)
			if gotScore != tc.wantScore || gotRisk != tc.wantRisk {
				t.Fatalf("got score=%d risk=%s, want score=%d risk=%s", gotScore, gotRisk, tc.wantScore, tc.wantRisk)
			}
		})
	}
}

func TestUnscannedFileIsNotMarkedSafe(t *testing.T) {
	log := &logstore.BrowserAILog{
		Action:            "Allowed",
		RiskScore:         10,
		PredictiveRisk:    "LOW",
		PredictedCategory: "SAFE",
	}
	markUnscannedFilePrediction(log)
	if log.PredictedCategory != "UNSCANNED_FILE" || log.PredictiveRisk != "MEDIUM" || log.RiskScore != 50 {
		t.Fatalf("unscanned file was not labeled unknown: category=%s risk=%s score=%d", log.PredictedCategory, log.PredictiveRisk, log.RiskScore)
	}
}

func TestScanGuardWarningRemainsWarning(t *testing.T) {
	log := &logstore.BrowserAILog{}
	if !applyScanGuardPrediction(log, "Warned", "Warn rule") {
		t.Fatal("warning action was not handled")
	}
	if log.Action != "Warned" || log.PredictedCategory != "AI_GUARD_BOT_WARNING" || log.PredictiveRisk != "LOW" || log.RiskScore != 40 {
		t.Fatalf("warning action was misclassified: action=%s category=%s risk=%s score=%d", log.Action, log.PredictedCategory, log.PredictiveRisk, log.RiskScore)
	}
}

func TestUnknownScanGuardActionIsNotAllowed(t *testing.T) {
	log := &logstore.BrowserAILog{Action: "Blocked", PredictedCategory: "SECURITY_POLICY_VIOLATION"}
	if applyScanGuardPrediction(log, "unexpected", "") {
		t.Fatal("unknown scan action was accepted")
	}
	if log.Action != "Blocked" || log.PredictedCategory != "SECURITY_POLICY_VIOLATION" {
		t.Fatalf("unknown action changed the existing verdict: action=%s category=%s", log.Action, log.PredictedCategory)
	}
}

func TestEvalErrorIsNotMarkedLowRisk(t *testing.T) {
	log := &logstore.BrowserAILog{RiskScore: 10, PredictiveRisk: "LOW"}
	markAIBotEvalErrorPrediction(log)
	if log.RiskScore != 50 || log.PredictiveRisk != "MEDIUM" {
		t.Fatalf("evaluation error remained low risk: risk=%s score=%d", log.PredictiveRisk, log.RiskScore)
	}
}
