//go:build lenscanary

package lensclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const lensSchemaEndpoint = "https://api.silica-prod01.io.lens.poly.com/graphql"

func TestLensGraphQLVariableTypes(t *testing.T) {
	t.Parallel()
	useWave1Types := os.Getenv("POLYLENS2OTEL_LENS_CANARY_USE_WAVE1_TYPES") == "true"
	tests := []struct {
		name        string
		variable    string
		correctType string
		wrongType   string
		correct     string
		wrong       string
		variables   map[string]string
	}{
		{
			name: "active calls device ID", variable: "deviceID", correctType: "String!", wrongType: "ID!",
			correct:   `query Canary($deviceID: String!) { activeCalls(deviceId: $deviceID) { calls { id } } }`,
			wrong:     `query Canary($deviceID: ID!) { activeCalls(deviceId: $deviceID) { calls { id } } }`,
			variables: map[string]string{"deviceID": "canary"},
		},
		{
			name: "CDR tenant ID", variable: "tenantID", correctType: "String!", wrongType: "ID!",
			correct: `query Canary($tenantID: String!) {
  meetingRecordLists(
    tenantId: [$tenantID]
    meetingRecordType: [CDR]
    timeRange: {relativeRange: {increment: DAY, value: 2}}
    first: 1
  ) { edges { node { deviceId } } }
}`,
			wrong: `query Canary($tenantID: ID!) {
  meetingRecordLists(
    tenantId: [$tenantID]
    meetingRecordType: [CDR]
    timeRange: {relativeRange: {increment: DAY, value: 2}}
    first: 1
  ) { edges { node { deviceId } } }
}`,
			variables: map[string]string{"tenantID": "canary"},
		},
		{
			name: "firmware PID", variable: "pid", correctType: "ID!", wrongType: "String!",
			correct:   `query Canary($pid: ID!) { availableProductSoftwareByPid(pid: $pid) { version } }`,
			wrong:     `query Canary($pid: String!) { availableProductSoftwareByPid(pid: $pid) { version } }`,
			variables: map[string]string{"pid": "canary"},
		},
	}
	client := &http.Client{Timeout: 15 * time.Second}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tt.correct
			if useWave1Types {
				query = tt.wrong
			}
			errors := queryLensSchema(t, client, query, tt.variables)
			for _, message := range errors {
				if message != "You must be logged in to run this query" && message != "Authorization failed" {
					t.Errorf("correct %s declaration was not accepted: %s", tt.correctType, message)
				}
			}

			wrongErrors := queryLensSchema(t, client, tt.wrong, tt.variables)
			want := fmt.Sprintf(`Variable "$%s" of type "%s" used in position expecting type "%s".`, tt.variable, tt.wrongType, tt.correctType)
			if !containsString(wrongErrors, want) {
				t.Fatalf("wrong type was not rejected with %q; errors=%q", want, wrongErrors)
			}
			t.Logf("accepted %s and rejected historical %s: %s", tt.correctType, tt.wrongType, want)
		})
	}
}

func queryLensSchema(t *testing.T, client *http.Client, query string, variables map[string]string) []string {
	t.Helper()
	body, err := json.Marshal(struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lensSchemaEndpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if (resp.StatusCode < 200 || resp.StatusCode >= 300) && len(envelope.Errors) == 0 {
		t.Fatalf("Lens schema endpoint returned unexplained HTTP %d", resp.StatusCode)
	}
	messages := make([]string, 0, len(envelope.Errors))
	for _, graphqlError := range envelope.Errors {
		messages = append(messages, strings.TrimSpace(graphqlError.Message))
	}
	return messages
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
