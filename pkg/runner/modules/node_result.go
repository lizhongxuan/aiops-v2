package modules

import (
	"encoding/json"

	"runner/noderesult"
)

func AttachNodeResult(output map[string]any, stdoutText string) {
	if output == nil {
		return
	}
	env, ok, err := noderesult.ParseStdout(stdoutText)
	if err != nil || !ok {
		return
	}
	var record map[string]any
	data, marshalErr := json.Marshal(env)
	if marshalErr != nil {
		return
	}
	if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
		return
	}
	output["node_result"] = record
	if len(env.Outputs) > 0 {
		output["outputs"] = env.Outputs
	}
	if len(env.Metrics) > 0 {
		output["metrics"] = env.Metrics
	}
}
