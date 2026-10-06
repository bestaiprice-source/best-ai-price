package storage

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
)

// LogEvent writes an append-only event to the ash_events table
func (s *Storage) LogEvent(ctx context.Context, ownerID string, sessionID, nodeID *string, eventType string, payload map[string]any) (*ent.AshEvent, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	builder := s.db.AshEvent.Create().
		SetOwnerID(ownerID).
		SetEventType(eventType).
		SetPayload(payload)

	if sessionID != nil {
		builder.SetSessionID(*sessionID)
	}
	if nodeID != nil {
		builder.SetNodeID(*nodeID)
	}

	return builder.Save(ctx)
}

// RecordUsage writes usage details to the ash_usages table
func (s *Storage) RecordUsage(ctx context.Context, ownerID string, sessionID, nodeID *string, inputTokens, outputTokens, costUnits int64) (*ent.AshUsage, error) {
	return s.RecordUsageDetails(ctx, ownerID, sessionID, nodeID, inputTokens, outputTokens, costUnits, "standard", "standard", "", "tokens", 0)
}

func (s *Storage) RecordUsageDetails(ctx context.Context, ownerID string, sessionID, nodeID *string, inputTokens, outputTokens, costUnits int64, poolType, provider, model, usageUnit string, billedMicroUSD int64) (*ent.AshUsage, error) {
	if s.db == nil {
		return nil, nil
	}

	builder := s.db.AshUsage.Create().
		SetOwnerID(ownerID).
		SetInputTokens(inputTokens).
		SetOutputTokens(outputTokens).
		SetCostUnits(costUnits).
		SetPoolType(poolType).SetProvider(provider).SetModel(model).SetUsageUnit(usageUnit).SetBilledMicroUsd(billedMicroUSD)

	if sessionID != nil {
		builder.SetSessionID(*sessionID)
	}
	if nodeID != nil {
		builder.SetNodeID(*nodeID)
	}

	return builder.Save(ctx)
}

// RecordUpstreamFailoverLoss records upstream cost for a failed request attempt to usage_logs with zero client charge (actual_cost = 0)
func (s *Storage) RecordUpstreamFailoverLoss(
	ctx context.Context,
	userID int64,
	accountID int64,
	modelName string,
	upstreamModel string,
	inputTokens int,
	outputTokens int,
	inputCostUSD float64,
	outputCostUSD float64,
	totalCostUSD float64,
	requestID string,
	stream bool,
) (*ent.UsageLog, error) {
	if s == nil || s.db == nil || accountID <= 0 {
		return nil, nil
	}

	// Look up an apiKeyID for this user to satisfy foreign key requirement
	var apiKeyID int64
	if userID > 0 {
		if firstKey, err := s.db.APIKey.Query().Where(apikey.UserIDEQ(userID)).First(ctx); err == nil && firstKey != nil {
			apiKeyID = firstKey.ID
		}
	}
	if apiKeyID <= 0 {
		if anyKey, err := s.db.APIKey.Query().First(ctx); err == nil && anyKey != nil {
			apiKeyID = anyKey.ID
			if userID <= 0 {
				userID = anyKey.UserID
			}
		}
	}

	// If still no valid user or API key in DB (e.g. test env without accounts/keys), skip DB insert gracefully
	if userID <= 0 || apiKeyID <= 0 {
		return nil, nil
	}

	builder := s.db.UsageLog.Create().
		SetUserID(userID).
		SetAPIKeyID(apiKeyID).
		SetAccountID(accountID).
		SetRequestID(requestID).
		SetModel(modelName).
		SetRequestedModel(modelName).
		SetUpstreamModel(upstreamModel).
		SetInputTokens(inputTokens).
		SetOutputTokens(outputTokens).
		SetInputCost(inputCostUSD).
		SetOutputCost(outputCostUSD).
		SetTotalCost(totalCostUSD).
		SetActualCost(0.0). // Crucial: zero client charge for failed failover attempt!
		SetRateMultiplier(1.0).
		SetBillingType(0).
		SetStream(stream).
		SetUserAgent("upstream_failover_loss").
		SetCreatedAt(time.Now())

	return builder.Save(ctx)
}
