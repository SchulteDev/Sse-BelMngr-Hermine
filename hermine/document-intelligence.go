package hermine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// diModelID See https://learn.microsoft.com/en-us/azure/ai-services/document-intelligence/model-overview for a list of all models.
	diModelID = "prebuilt-invoice"

	diPollingInterval = 1 * time.Second
	diAPIVersion      = "2024-11-30"
)

func enqueueAnalysisAndWaitForCompletion(logger *log.Entry, diEndpoint, diKey, pathOfFileToImport string) (*diAnalyzeResult, error) {
	hc := &http.Client{Timeout: 30 * time.Second}

	runningAnalysisURL, enqueueErr := enqueueAnalysis(logger, hc, diEndpoint, diKey, pathOfFileToImport)
	if enqueueErr != nil {
		return nil, enqueueErr
	}

	pollResult, pollErr := pollUntilCompletion(logger, hc, runningAnalysisURL, diKey)
	if pollErr != nil {
		return nil, pollErr
	}
	if pollResult.AnalyzeResult == nil {
		noResultErr := errors.New("AnalyzeResult is missing")
		logger.WithError(noResultErr).Warn()
		return nil, noResultErr
	}
	logger.Debugf("Analysis done, contains %d document(s)", len(pollResult.AnalyzeResult.Documents))

	return pollResult.AnalyzeResult, nil
}

func enqueueAnalysis(logger *log.Entry, hc *http.Client, diEndpoint, diKey, pathOfFileToImport string) (*url.URL, error) {
	req, newReqErr := newAnalyzeHTTPRequest(logger, diEndpoint, diKey, pathOfFileToImport)
	if newReqErr != nil {
		return nil, newReqErr
	}

	logger.Trace("Submitting file for DI analysis...")
	resp, respErr := hc.Do(req)
	if respErr != nil {
		logger.WithError(respErr).Debug("Failed to execute HTTP request")
		return nil, respErr
	}
	defer closeBody(logger, resp)

	if resp.StatusCode != http.StatusAccepted {
		responseBody, responseBodyErr := io.ReadAll(resp.Body)
		restCallFailedErr := fmt.Errorf("rest call failed, status: %d, error message: %s", resp.StatusCode, responseBody)
		logger.
			WithField("read_response_body_error", responseBodyErr).
			WithError(restCallFailedErr).Debug()

		return nil, restCallFailedErr
	}

	operationLocation := resp.Header.Get("Operation-Location")
	logger.WithField("operation_location", operationLocation).Debug()

	return url.Parse(operationLocation)
}

func newAnalyzeHTTPRequest(logger *log.Entry, diEndpoint, diKey, pathOfFileToImport string) (*http.Request, error) {
	fileContent, readFileErr := os.ReadFile(pathOfFileToImport)
	if readFileErr != nil {
		logger.WithError(readFileErr).Warn("Failed to read file")
		return nil, readFileErr
	}

	diURL := fmt.Sprintf("%s/documentintelligence/documentModels/%s:analyze?api-version=%s", diEndpoint, diModelID, diAPIVersion)
	req, newRequestErr := http.NewRequest(http.MethodPost, diURL, strings.NewReader(string(fileContent)))
	if newRequestErr != nil {
		logger.WithError(newRequestErr).Warn("Failed to create a new HTTP request")
		return nil, newRequestErr
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	addDiAuthenticationHeader(req, diKey)

	return req, nil
}

func pollUntilCompletion(logger *log.Entry, hc *http.Client, pollingURL *url.URL, diKey string) (*diAnalysisStatus, error) {
	req, newRequestErr := http.NewRequest(http.MethodGet, pollingURL.String(), nil)
	if newRequestErr != nil {
		createErr := fmt.Errorf("failed to create a new HTTP GET request: %w", newRequestErr)
		logger.WithError(newRequestErr).Warn(createErr)
		return nil, createErr
	}
	addDiAuthenticationHeader(req, diKey)

	for {
		if diStatus, err := pollAnalysisStatus(logger, hc, req); diStatus != nil && err == nil {
			return diStatus, nil
		}

		time.Sleep(diPollingInterval)
	}
}

func pollAnalysisStatus(logger *log.Entry, hc *http.Client, req *http.Request) (*diAnalysisStatus, error) {
	resp, respErr := hc.Do(req)
	if respErr != nil {
		logger.WithError(respErr).Warn("Failed to perform HTTP GET during polling, trying again...")
		return nil, respErr
	}
	defer closeBody(logger, resp)

	switch resp.StatusCode {
	case http.StatusOK:
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logger.WithError(readErr).Warn("Failed to read response body")
			return nil, readErr
		}

		var analysisStatus diAnalysisStatus
		if unmarshalErr := json.Unmarshal(body, &analysisStatus); unmarshalErr != nil {
			logger.WithError(unmarshalErr).Warn("Failed to parse response body into structured JSON")
			return nil, unmarshalErr
		}

		switch {
		case analysisStatus.isStatusSucceeded():
			logger.Debug("Analysis succeeded")
			return &analysisStatus, nil
		case analysisStatus.isStatusRunning():
			logger.Trace("Analysis running...")
		default:
			logger.Warnf("Unexpected response body: %s, trying again...", body)
		}
	case http.StatusTooManyRequests:
		logger.Debugf("Too many requests, trying again...")
	default:
		logger.Debugf("Unexpected status code received: %d, trying again...", resp.StatusCode)
	}

	return nil, nil
}

func addDiAuthenticationHeader(req *http.Request, diKey string) {
	req.Header.Set("Ocp-Apim-Subscription-Key", diKey)
}

func closeBody(logger *log.Entry, r *http.Response) {
	if err := r.Body.Close(); err != nil {
		logger.WithError(err).Debug("Failed to close HTTP response body")
	}
}
