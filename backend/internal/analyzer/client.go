package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aca/backend/internal/config"
	"aca/backend/internal/models"
	"aca/backend/internal/scoring"
)

type Client struct {
	cfg     config.Config
	client  *http.Client
	scoring *scoring.Service
}

func New(cfg config.Config, scoringService *scoring.Service) *Client {
	return &Client{cfg: cfg, client: &http.Client{Timeout: 45 * time.Second}, scoring: scoringService}
}

func (c *Client) Analyze(ctx context.Context, session models.Session) models.ReviewReport {
	base := scoring.FallbackReview(session)
	if c.cfg.CPPAnalyzerURL != "" {
		if report, err := c.callCPP(ctx, session); err == nil && report.TotalScore > 0 && len(report.Dimensions) > 0 {
			return report
		}
	}
	return c.scoring.ImproveReviewWithModels(ctx, session, base)
}

func (c *Client) callCPP(ctx context.Context, session models.Session) (models.ReviewReport, error) {
	body, _ := json.Marshal(session)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.CPPAnalyzerURL+"/analyze", bytes.NewReader(body))
	if err != nil {
		return models.ReviewReport{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return models.ReviewReport{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return models.ReviewReport{}, fmt.Errorf("cpp analyzer status %d", resp.StatusCode)
	}
	var out models.ReviewReport
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return models.ReviewReport{}, err
	}
	if out.ReportID == "" {
		out.ReportID = "report-" + session.ID
	}
	if out.SessionID == "" {
		out.SessionID = session.ID
	}
	return out, nil
}
