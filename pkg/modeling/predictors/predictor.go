package predictors

import "context"

type FeatureSet struct {
	HistoryWindow int               `json:"history_window"`
	Horizon       int               `json:"horizon"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type PredictionResult struct {
	ModelName string             `json:"model_name"`
	Outputs   map[string]float64 `json:"outputs,omitempty"`
}

type Predictor interface {
	Name() string
	Predict(ctx context.Context, input FeatureSet) (*PredictionResult, error)
}
