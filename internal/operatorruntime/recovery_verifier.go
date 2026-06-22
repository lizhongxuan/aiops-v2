package operatorruntime

import (
	"context"
	"time"
)

type RecoveryVerification struct {
	Recovered bool    `json:"recovered"`
	Reason    string  `json:"reason,omitempty"`
	FinalLag  float64 `json:"finalLag,omitempty"`
}

type RecoveryVerifier struct {
	inspector InspectionRunner
	sleep     func(context.Context, time.Duration) error
}

type RecoveryVerifierOption func(*RecoveryVerifier)

func WithRecoveryVerifierSleeper(sleeper func(context.Context, time.Duration) error) RecoveryVerifierOption {
	return func(v *RecoveryVerifier) {
		v.sleep = sleeper
	}
}

func NewRecoveryVerifier(inspector InspectionRunner, opts ...RecoveryVerifierOption) *RecoveryVerifier {
	v := &RecoveryVerifier{
		inspector: inspector,
		sleep: func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func (v *RecoveryVerifier) Verify(ctx context.Context, cluster PGCluster, template InspectionTemplate, replica PGInstance, policy VerifyPolicy) (RecoveryVerification, error) {
	cluster = NormalizeResource(cluster)
	if policy.TimeoutSeconds <= 0 {
		policy.TimeoutSeconds = 300
	}
	if policy.IntervalSeconds <= 0 {
		policy.IntervalSeconds = 30
	}
	attempts := policy.TimeoutSeconds/policy.IntervalSeconds + 1
	for attempt := 0; attempt < attempts; attempt++ {
		results, err := v.inspector.Inspect(ctx, cluster, template)
		if err != nil {
			return RecoveryVerification{Recovered: false, Reason: err.Error()}, nil
		}
		for _, result := range results {
			targetID := result.TargetID
			if targetID == "" {
				targetID = result.ReplicaID
			}
			if targetID != replica.ID {
				continue
			}
			receiver := result.Fields[FieldReplicaReceiverRunning]
			lag := result.Fields[FieldReplicaReplayLagSeconds]
			if policy.ReceiverRunningRequired && (!receiver.Known || !receiver.Bool) {
				return RecoveryVerification{Recovered: false, Reason: "receiver is not running"}, nil
			}
			if !lag.Known {
				return RecoveryVerification{Recovered: false, Reason: "replay lag is unknown"}, nil
			}
			if lag.Number < float64(policy.MaxReplayLagSeconds) {
				return RecoveryVerification{Recovered: true, FinalLag: lag.Number}, nil
			}
		}
		if attempt < attempts-1 {
			if err := v.sleep(ctx, time.Duration(policy.IntervalSeconds)*time.Second); err != nil {
				return RecoveryVerification{Recovered: false, Reason: err.Error()}, err
			}
		}
	}
	return RecoveryVerification{Recovered: false, Reason: "verification timeout"}, nil
}
