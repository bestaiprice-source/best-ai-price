package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/providerpopularity"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type providerPopularityRepository struct {
	client *dbent.Client
}

func NewProviderPopularityRepository(client *dbent.Client) service.ProviderPopularityRepository {
	return &providerPopularityRepository{client: client}
}

func (r *providerPopularityRepository) IncrementClick(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()

	err := client.ProviderPopularity.
		Create().
		SetProvider(provider).
		SetClickCount(1).
		SetLastClickedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		OnConflictColumns(providerpopularity.FieldProvider).
		Update(func(u *dbent.ProviderPopularityUpsert) {
			u.AddClickCount(1)
			u.SetLastClickedAt(now)
			u.SetUpdatedAt(now)
		}).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return r.GetByProvider(ctx, provider)
}

func (r *providerPopularityRepository) GetByProvider(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	entItem, err := r.client.ProviderPopularity.
		Query().
		Where(providerpopularity.ProviderEQ(provider)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return &service.ProviderPopularity{
				Provider:   provider,
				ClickCount: 0,
			}, nil
		}
		return nil, err
	}

	return &service.ProviderPopularity{
		ID:            entItem.ID,
		Provider:      entItem.Provider,
		ClickCount:    entItem.ClickCount,
		LastClickedAt: entItem.LastClickedAt,
		CreatedAt:     entItem.CreatedAt,
		UpdatedAt:     entItem.UpdatedAt,
	}, nil
}

func (r *providerPopularityRepository) ListPopularity(ctx context.Context) ([]service.ProviderPopularityItem, error) {
	items, err := r.client.ProviderPopularity.
		Query().
		Order(dbent.Desc(providerpopularity.FieldClickCount), dbent.Desc(providerpopularity.FieldLastClickedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.ProviderPopularityItem, len(items))
	for i, item := range items {
		out[i] = service.ProviderPopularityItem{
			Provider:      item.Provider,
			ClickCount:    item.ClickCount,
			Rank:          i + 1,
			LastClickedAt: item.LastClickedAt,
		}
	}
	return out, nil
}
