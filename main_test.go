package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRedactPHI(t *testing.T) {
	task := Task{
		ID:          "test1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName":     "Alice",
			"dob":             "10/09/1991",
			"phone":           "2024157890",
			"insuranceNumber": "138928943893",
		},
	}

	concealPHI(&task)

	if task.Payload["patientName"] != "[CONCEALED]" {
		t.Errorf("Expected patient name to be concealed, got %v", task.Payload["patientName"])
	}

	if task.Payload["dob"] != "[CONCEALED]" {
		t.Errorf("Date of Birth should be concealed.")
	}

	if task.Payload["phone"] != "[CONCEALED]" {
		t.Errorf("Phone number should be concealed.")
	}

	if task.Payload["insuranceNumber"] != "[CONCEALED]" {
		t.Errorf("Insurance number should be concealed.")
	}
}

func TestRedactPHI2(t *testing.T) {
	task := Task{
		ID:          "test2",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Daniel",
			"diagnosis":   "Bronchitis",
			"address":     "2890 Halfview Court San Bruno, CA",
		},
	}

	concealPHI(&task)

	if task.Payload["diagnosis"] != "[CONCEALED]" {
		t.Errorf("Expected patient diagnosis to be concealed, got %v", task.Payload["diagnosis"])
	}
}

func TestRedactPHI_NoPHI(t *testing.T) {
	task := Task{
		ID:          "test2",
		ContainsPHI: false,
		Payload: map[string]interface{}{
			"patientName": "Bob",
		},
	}

	concealPHI(&task)

	if task.Payload["patientName"] != "Bob" {
		t.Errorf("Expected patientName to remain unchanged, got %v", task.Payload["patientName"])
	}
}

func TestAuditLog_Success(t *testing.T) {
	task := Task{
		ID:          "random_test",
		ContainsPHI: false,
		Payload: map[string]interface{}{
			"patientName": "Randy",
		},
	}

	logFile := "taskbeat_audit.log"
	os.Remove(logFile)
	defer os.Remove(logFile)

	err := auditLog(task)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Could not read log file: %v", err)
	}

	if !strings.Contains(string(data), "TaskID=random_test") {
		t.Errorf("Log entry missing expected task ID: got %s", string(data))
	}
}

func TestQueueHandler_Success(t *testing.T) {
	task := Task{
		ID:          "task1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Angela",
			"phone":       "9491203489",
		},
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	queueHandler(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", result.StatusCode)
	}
	respBody, _ := io.ReadAll(result.Body)
	var returned Task
	err := json.Unmarshal(respBody, &returned)
	if err != nil {
		t.Fatalf("Response is not a valid JSON: %v", err)
	}

	if returned.ID != task.ID {
		t.Errorf("Expected ID %s, got %s", task.ID, returned.ID)
	}

	if returned.Payload["patientName"] != "[CONCEALED]" {
		t.Errorf("Expected patient name to be concealed but got %v", returned.Payload["patientName"])
	}
}

func TestQueueHandler_Failure(t *testing.T) {
	task := Task{
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Brad",
		},
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	queueHandler(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)
	if !strings.Contains(string(respBody), "Missing") {
		t.Errorf("Didn't get the expected error message")
	}
}

func TestQueueHandler_JSONFailure(t *testing.T) {
	invalidJSON := "har har"
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader([]byte(invalidJSON)))
	rec := httptest.NewRecorder()

	queueHandler(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)
	if !strings.Contains(string(respBody), "Invalid") {
		t.Errorf("Didn't get the expected error message")
	}
}
