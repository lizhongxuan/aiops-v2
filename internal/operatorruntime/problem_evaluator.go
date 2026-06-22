package operatorruntime

import (
	"fmt"
	"time"
)

type ProblemEvaluator struct {
	now        func() time.Time
	sinceByKey map[string]time.Time
}

type ProblemEvaluation struct {
	Matches         []ProblemMatch `json:"matches,omitempty"`
	MissingEvidence []string       `json:"missingEvidence,omitempty"`
}

type ProblemMatch struct {
	ProblemTypeID      string    `json:"problemTypeId"`
	ProblemDisplayName string    `json:"problemDisplayName,omitempty"`
	ReplicaID          string    `json:"replicaId,omitempty"`
	Since              time.Time `json:"since"`
	AutoRepairAllowed  bool      `json:"autoRepairAllowed"`
}

func NewProblemEvaluator(now func() time.Time) *ProblemEvaluator {
	if now == nil {
		now = time.Now
	}
	return &ProblemEvaluator{now: now, sinceByKey: map[string]time.Time{}}
}

func (e *ProblemEvaluator) Evaluate(problem ProblemType, result InspectionResult) ProblemEvaluation {
	resourceID := result.ResourceID
	if resourceID == "" {
		resourceID = result.ClusterID
	}
	targetID := result.TargetID
	if targetID == "" {
		targetID = result.ReplicaID
	}
	key := fmt.Sprintf("%s/%s/%s", problem.ID, resourceID, targetID)
	allTrue := true
	var missing []string
	for _, condition := range problem.Conditions {
		actual, ok := result.Fields[condition.Field]
		if !ok || !actual.Known {
			allTrue = false
			missing = append(missing, condition.Field)
			continue
		}
		if !compareFieldValue(actual, condition.Operator, condition.Value) {
			allTrue = false
		}
	}
	if !allTrue {
		delete(e.sinceByKey, key)
		return ProblemEvaluation{MissingEvidence: missing}
	}
	now := e.now()
	since, ok := e.sinceByKey[key]
	if !ok {
		e.sinceByKey[key] = now
		since = now
	}
	if int(now.Sub(since).Seconds()) < problem.ForSeconds {
		return ProblemEvaluation{}
	}
	return ProblemEvaluation{Matches: []ProblemMatch{{
		ProblemTypeID:      problem.ID,
		ProblemDisplayName: problem.DisplayName,
		ReplicaID:          targetID,
		Since:              since,
		AutoRepairAllowed:  problem.AutoRepairAllowed,
	}}}
}

func compareFieldValue(actual FieldValue, operator ProblemOperator, expected FieldValue) bool {
	switch actual.Type {
	case FieldTypeBool:
		switch operator {
		case OperatorEqual:
			return actual.Bool == expected.Bool
		case OperatorNotEqual:
			return actual.Bool != expected.Bool
		}
	case FieldTypeNumber:
		switch operator {
		case OperatorEqual:
			return actual.Number == expected.Number
		case OperatorNotEqual:
			return actual.Number != expected.Number
		case OperatorGreaterThan:
			return actual.Number > expected.Number
		case OperatorGreaterThanOrEqual:
			return actual.Number >= expected.Number
		case OperatorLessThan:
			return actual.Number < expected.Number
		case OperatorLessThanOrEqual:
			return actual.Number <= expected.Number
		}
	case FieldTypeString:
		switch operator {
		case OperatorEqual:
			return actual.String == expected.String
		case OperatorNotEqual:
			return actual.String != expected.String
		}
	}
	return false
}
