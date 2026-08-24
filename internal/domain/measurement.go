package domain

type MeasurementPoint struct {
	ID             string   `json:"id"`
	TaskID         string   `json:"taskId"`
	Revision       uint64   `json:"revision"`
	PointLabel     string   `json:"pointLabel"`
	ReferenceValue float64  `json:"referenceValue"`
	ObservedValue  *float64 `json:"observedValue,omitempty"`
	Unit           string   `json:"unit"`
	Uncertainty    float64  `json:"uncertainty"`
	Tolerance      float64  `json:"tolerance"`
	Error          float64  `json:"error"`
	Judgement      string   `json:"judgement"`
	EnteredBy      string   `json:"enteredBy"`
}

type CoverageItem struct {
	Label          string   `json:"label"`
	Value          float64  `json:"value"`
	MeasurementIDs []string `json:"measurementIds,omitempty"`
	Complete       bool     `json:"complete"`
	Reason         string   `json:"reason,omitempty"`
}

type CoverageSnapshot struct {
	Required  int            `json:"required"`
	Completed int            `json:"completed"`
	Rate      float64        `json:"rate"`
	Complete  bool           `json:"complete"`
	Covered   []CoverageItem `json:"covered"`
	Missing   []CoverageItem `json:"missing"`
	Duplicate []CoverageItem `json:"duplicate"`
}

type MeasurementSnapshot struct {
	Count      int    `json:"count"`
	Qualified  int    `json:"qualified"`
	Deviations int    `json:"deviations"`
	Complete   bool   `json:"complete"`
	Overall    string `json:"overall"`
}
