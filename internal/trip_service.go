package tripservice

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"
	api "github.com/saywich/tripgo/internal/generated"
)

var (
	ErrTripNotFound = errors.New("trip not found")
	ErrTripAlreadyCompleted = errors.New("trip already completed")
	ErrDriverBusy = errors.New("driver already has an active trip")
)

type tripCreator interface {
	Create(
		ctx context.Context,
		data api.TripData,
	) (api.Trip, error)
	SearchById(ctx context.Context, tripId openapi_types.UUID) (api.Trip, error)
	Finish(ctx context.Context, tripId openapi_types.UUID) (api.Trip, error)
}

type tripStatusHistoryCreator interface {
	Create(
		ctx context.Context,
		tripId openapi_types.UUID,
		fromStatus api.TripStatus,
		toStatus api.TripStatus,
		reason string,
	) error
}

type TripService struct {
	txManager                   TxManager
	tripRepository              tripCreator
	tripStatusHistoryRepository tripStatusHistoryCreator
}

func NewTripService(
	txManager TxManager,
	tripRepository tripCreator,
	tripStatusHistoryRepository tripStatusHistoryCreator,
) *TripService {
	return &TripService{
		txManager:                   txManager,
		tripRepository:              tripRepository,
		tripStatusHistoryRepository: tripStatusHistoryRepository,
	}
}

func (s *TripService) Create(
	ctx context.Context,
	data api.TripData,
) (api.Trip, error) {
	var trip api.Trip

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		createdTrip, err := s.tripRepository.Create(ctx, data)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && 
				pgErr.Code == "23505" && 
				pgErr.ConstraintName == "trips_one_active_trip_per_driver_idx" {
				return ErrDriverBusy
			}

			return err
		}

		err = s.tripStatusHistoryRepository.Create(
			ctx,
			createdTrip.Id,
			createdTrip.Status,
			api.Active,
			"Trip created",
		)
		if err != nil {
			return err
		}

		trip = createdTrip

		return nil
	})

	if err != nil {
		return api.Trip{}, err
	}

	return trip, nil
}

func (s *TripService) SearchById(ctx context.Context, tripId openapi_types.UUID) (api.Trip, error) {
	return s.tripRepository.SearchById(ctx, tripId)
}

func (s *TripService) Finish(
	ctx context.Context,
	tripId openapi_types.UUID,
) (api.Trip, error) {
	var trip api.Trip

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		finishedTrip, err := s.tripRepository.Finish(ctx, tripId)
		if errors.Is(err, pgx.ErrNoRows) {
			currentTrip, searchErr := s.tripRepository.SearchById(ctx, tripId)
			if errors.Is(searchErr, pgx.ErrNoRows) {
				return ErrTripNotFound
			}
			if searchErr != nil {
				return searchErr
			}
			if currentTrip.Status == api.Completed {
				return ErrTripAlreadyCompleted
			}
			return ErrTripNotFound
		}

		if err != nil {
			return err
		}

		if err := s.tripStatusHistoryRepository.Create(
			ctx, finishedTrip.Id, api.Active, api.Completed, "Trip finished",
		); err != nil {
			return err
		}

		trip = finishedTrip

		return nil
	})

	if err != nil {
		return api.Trip{}, err
	}

	return trip, nil
}
