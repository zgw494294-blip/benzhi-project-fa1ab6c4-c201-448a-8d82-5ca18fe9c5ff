package domain

// The domain model is split by aggregate concern across the files in this
// package. Keeping the types together as one package preserves the original
// JSON contracts while making the layer boundaries explicit.

type TaskStatus string

const (
	StatusPending  TaskStatus = "待测"
	StatusReview   TaskStatus = "待复核"
	StatusReturned TaskStatus = "已退回"
	StatusFrozen   TaskStatus = "已冻结"
)
