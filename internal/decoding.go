package tripservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	api "github.com/saywich/tripgo/internal/generated"
)

func decodeTripData(body io.Reader) (api.TripData, error) {
	rawJson, err := decodeRawSingleObject(body)
	if err != nil {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}
	
	jsonMap, err := decodeWithRequiredFields(
		rawJson, "driver_id", "user_id", "start_point", "end_point", "price")
	if err != nil {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}
	if _, err = decodeCoordinates(bytes.NewReader(jsonMap["start_point"])); err != nil {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}
	if _, err = decodeCoordinates(bytes.NewReader(jsonMap["end_point"])); err != nil {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}

	var tripData api.TripData

	decoder := json.NewDecoder(bytes.NewReader(rawJson))
	decoder.DisallowUnknownFields()

	if decodeErr := decoder.Decode(&tripData); decodeErr != nil {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: %w", decodeErr)
	}

	if tripData.DriverId == uuid.Nil || tripData.UserId == uuid.Nil || tripData.Price < 0 {
		return api.TripData{}, fmt.Errorf("Error decoding JSON body: invalid field value")
	}

	return tripData, nil
}

func decodeCoordinates(body io.Reader) (api.Coordinates, error) {
	rawJson, err := decodeRawSingleObject(body)
	if err != nil {
		return api.Coordinates{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}

	if _, err = decodeWithRequiredFields(rawJson, "latitude", "longitude"); err != nil {
		return api.Coordinates{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}

	var coordinates api.Coordinates
	decoder := json.NewDecoder(bytes.NewReader(rawJson))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&coordinates); err != nil {
		return api.Coordinates{}, fmt.Errorf("Error decoding JSON body: %w", err)
	}

	if coordinates.Latitude < -90 || coordinates.Latitude > 90 ||
		coordinates.Longitude < -180 || coordinates.Longitude > 180 {
		return api.Coordinates{}, fmt.Errorf("Error decoding JSON body: invalid coordinates")
	}
 
	return coordinates, nil
}

func decodeWithRequiredFields(
	raw json.RawMessage,
	fields ...string,
) (map[string]json.RawMessage, error) {
	var jsonMap map[string]json.RawMessage

	if err := json.Unmarshal(raw, &jsonMap); err != nil || jsonMap == nil {
		return nil, fmt.Errorf("JSON object required")
	}

	for _, field := range fields {
		value, ok := jsonMap[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%q is required", field)
		}
	}

	return jsonMap, nil
}

func decodeRawSingleObject(body io.Reader) (json.RawMessage, error) {
	var rawJson json.RawMessage

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&rawJson); err != nil {
		return json.RawMessage{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return json.RawMessage{}, fmt.Errorf("body must contain one JSON object")
	}

	return rawJson, nil
}
