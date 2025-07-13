package core

import (
	"QuickPort/tray"
	"encoding/json"
)

func ConvertMapToFileReqestMeta(input any) (*fileRequestData, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta fileRequestData
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func ConvertMapToFileMeta(input any) (*[]tray.FileMeta, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta []tray.FileMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func ConvertMapToAuthMeta(input any) (*tray.AuthMeta, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta tray.AuthMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// Helper functions for data conversion
func convertMapToMissingPacketData(input interface{}) (*MissingPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data MissingPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func convertMapToFinishPacketData(input interface{}) (*FinishPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data FinishPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func convertMapToErrorPacketData(input interface{}) (*ErrorPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data ErrorPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
