//go:build !pig_strip_bedrock_converse_stream

package bedrock

import (
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestBedrockFailureRetainsRecoveryDeadline(t *testing.T) {
	before := time.Now().UnixMilli()
	err := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"3"}}}},
		Err:      &smithy.GenericAPIError{Code: "ThrottlingException", Message: "rate limited"},
	}
	builder := ai.NewAssistantStreamBuilder(t.Context(), ai.APIBedrockConverseStream, "amazon-bedrock", "fixture")
	failBedrockResponse(t.Context(), builder, err, "fixture", false)
	message := builder.Stream().Result()
	failure := ai.ProviderFailureForMessage(*message)
	if failure.Kind != "rate_limit" || failure.Status != 429 || failure.RetryAt < before+3000 {
		t.Fatalf("recovery metadata lost: %+v", failure)
	}
	if newBedrockRetryer(aws.RetryModeStandard).IsErrorRetryable(err) {
		t.Fatal("timed rejection must reach the visible session wait rather than an SDK retry")
	}
}

func TestBedrockQuotaIsNotRetriedBySDK(t *testing.T) {
	err := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: 429}},
		Err:      &smithy.GenericAPIError{Code: "ServiceQuotaExceededException", Message: "quota exhausted"},
	}
	if newBedrockRetryer(aws.RetryModeStandard).IsErrorRetryable(err) {
		t.Fatal("SDK retried a quota rejection without a known reset")
	}
}
