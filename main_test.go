package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.etcd.io/bbolt"
	bolt "go.etcd.io/bbolt"
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

func TestAuditLog_Failure(t *testing.T) {
	task := Task{}
	err := auditLog(task)
	if err == nil {
		t.Fatalf("Expected an error due to no Task ID, but got nothing.")
	}
}

func TestAuditLog_Failure2(t *testing.T) {
	task := Task{
		ID: "test",
	}
	err := auditLog(task)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	data, err := os.ReadFile("random")
	if err == nil {
		t.Fatalf("Wasn't supposed to be able to read the given file, but got %s", data)
	}
}

func TestQueueHandler_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskQueue = make(chan Task, 1)
	handler := queueHandler(db, taskQueue)

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

	handler.ServeHTTP(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("Expected status 202 Accepted, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)
	var resp map[string]string
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp["status"] != "task queued" {
		t.Errorf("Expected task queued status, got %v", resp["status"])
	}

	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("Tasks"))
		v := b.Get([]byte("task1"))
		if v == nil {
			t.Fatalf("Task not found in DB")
		}

		var saved Task
		if err := json.Unmarshal(v, &saved); err != nil {
			t.Fatalf("Failed to unmarshal task from DB: %v", err)
		}

		if saved.Payload["patientName"] != "[CONCEALED]" {
			t.Errorf("Expected patient name to be concealed, got %v", saved.Payload["patientName"])
		}
		if saved.Payload["phone"] != "[CONCEALED]" {
			t.Errorf("Expected phone to be concealed, got %v", saved.Payload["phone"])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("DB view error: %v", err)
	}
}

func TestQueueHandler_Failure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskQueue = make(chan Task, 1)
	task := Task{
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Brad",
		},
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler := queueHandler(db, taskQueue)
	handler(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)
	if !strings.Contains(string(respBody), "missing") {
		t.Errorf("Didn't get the expected error message")
	}
}

func TestQueueHandler_JSONFailure(t *testing.T) {
	taskQueue = make(chan Task, 1)
	invalidJSON := "har har"
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader([]byte(invalidJSON)))
	rec := httptest.NewRecorder()

	handler := queueHandler(db, taskQueue)
	handler(rec, req)

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

func TestQueueHandlerBadPayload(t *testing.T) {
	taskQueue = make(chan Task, 1)
	task := Task{
		ID:          "task1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Brad",
			"pirate":      "har har har",
		},
	}
	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler := queueHandler(db, taskQueue)
	handler(rec, req)

	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)

	// fmt.Println(string(respBody))
	if !strings.Contains(string(respBody), "invalid key") {
		t.Errorf("Didn't get the expected error message")
	}

}

func setupTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	testDB, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}
	err = testDB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Tasks"))
		return err
	})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}
	return testDB
}

func TestQueueHandler_DBIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskQueue = make(chan Task, 1)

	task := Task{
		ID:          "t1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Angela",
			"phone":       "8089007171",
		},
		CreatedAt: time.Now(),
	}
	body, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler := queueHandler(db, taskQueue)
	handler(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted, got %d", res.StatusCode)
	}

	respBody, _ := io.ReadAll(res.Body)
	var resp map[string]string
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if resp["status"] != "task queued" {
		t.Errorf("Expected task queued status, got %v", resp["status"])
	}

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Tasks"))
		v := b.Get([]byte("t1"))
		if v == nil {
			t.Fatalf("Task not found in DB")
		}

		var saved Task
		if err := json.Unmarshal(v, &saved); err != nil {
			t.Fatalf("Failed to unmarshal task from DB: %v", err)
		}

		if saved.Payload["patientName"] != "[CONCEALED]" {
			t.Errorf("Expected patientName to be concealed, got %v", saved.Payload["patientName"])
		}
		if saved.Payload["phone"] != "[CONCEALED]" {
			t.Errorf("Expected phone to be concealed, got %v", saved.Payload["phone"])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("DB view error: %v", err)
	}
}

// func TestQueueHandler_Integration(t *testing.T) {
// 	db := TestDB(t)
// 	defer db.Close()

// 	handler := queueHandler(db)

// 	task := Task{
// 		ID:          "integration1",
// 		ContainsPHI: true,
// 		Payload: map[string]interface{}{
// 			"patientName": "Angela",
// 			"phone":       "9491203489",
// 		},
// 	}

// 	body, _ := json.Marshal(task)
// 	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
// 	rec := httptest.NewRecorder()

// 	handler.ServeHTTP(rec, req)
// 	result := rec.Result()
// 	defer result.Body.Close()

// 	if result.StatusCode != http.StatusOK {
// 		t.Fatalf("Expected status 200 OK, got %d", result.StatusCode)
// 	}

// 	respBody, _ := io.ReadAll(result.Body)
// 	var returned Task
// 	if err := json.Unmarshal(respBody, &returned); err != nil {
// 		t.Fatalf("Failed to unmarshall response: %v", err)
// 	}

// 	if returned.Payload["patientName"] != "[CONCEALED]" {
// 		t.Errorf("Patient Name was not concealed, got %v instead", returned.Payload["patientName"])
// 	}

// 	err := db.View(func(tx *bolt.Tx) error {
// 		b := tx.Bucket([]byte("Tasks"))
// 		v := b.Get([]byte("integration1"))
// 		if v == nil {
// 			t.Errorf("Task not found in DB")
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		t.Fatalf("DB view error: %v", err)
// 	}
// }

func TestQueueHandler_IntegrationII(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskQueue = make(chan Task, 10) // buffered to avoid blocking
	handler := queueHandler(db, taskQueue)

	task := Task{
		ID:          "integration2",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Angela",
			"phone":       "9491203489",
			"dob":         "6/7/2001",
			"diagnosis":   "Herpes",
		},
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check HTTP response
	result := rec.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("Expected status 202 Accepted, got %d", result.StatusCode)
	}

	respBody, _ := io.ReadAll(result.Body)
	var resp map[string]string
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp["status"] != "task queued" {
		t.Errorf("Expected task queued status, got %v", resp["status"])
	}

	// Check DB for saved task
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Tasks"))
		v := b.Get([]byte("integration2"))
		if v == nil {
			t.Fatalf("Task not found in DB")
		}

		var saved Task
		if err := json.Unmarshal(v, &saved); err != nil {
			t.Fatalf("Failed to unmarshal task from DB: %v", err)
		}

		// Check PHI redaction
		if saved.Payload["patientName"] != "[CONCEALED]" {
			t.Errorf("Expected patientName to be concealed, got %v", saved.Payload["patientName"])
		}
		if saved.Payload["dob"] != "[CONCEALED]" {
			t.Errorf("Expected dob to be concealed, got %v", saved.Payload["dob"])
		}
		if saved.Payload["diagnosis"] != "[CONCEALED]" {
			t.Errorf("Expected diagnosis to be concealed, got %v", saved.Payload["diagnosis"])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("DB view error: %v", err)
	}

	// Check that the task was actually enqueued
	select {
	case queuedTask := <-taskQueue:
		if queuedTask.ID != "integration2" {
			t.Errorf("Expected queued task ID integration2, got %v", queuedTask.ID)
		}
	default:
		t.Errorf("Task was not enqueued")
	}
}

// func TestQueueHandler_IntegrationFailure(t *testing.T) {
// 	db := TestDB(t)
// 	defer db.Close()

// 	handler := queueHandler(db)

// 	task := Task{
// 		ID:          "integration1",
// 		ContainsPHI: true,
// 		Payload: map[string]interface{}{
// 			"patientName": "Angela",
// 			"phone":       "9491203489",
// 			"garbageKey":  "garbage",
// 		},
// 	}

// 	body, _ := json.Marshal(task)
// 	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
// 	rec := httptest.NewRecorder()

// 	handler.ServeHTTP(rec, req)
// 	result := rec.Result()
// 	defer result.Body.Close()

// 	if result.StatusCode != http.StatusBadRequest {
// 		t.Fatalf("Expected status 400 Bad Request, got %d", result.StatusCode)
// 	}

// }

// func TestQueueHandler_IntegrationFailure2(t *testing.T) {
// 	db := TestDB(t)
// 	defer db.Close()

// 	handler := queueHandler(db)

// 	task := Task{
// 		ContainsPHI: true,
// 		Payload: map[string]interface{}{
// 			"patientName": "Angela",
// 			"phone":       "9491203489",
// 		},
// 	}

// 	body, _ := json.Marshal(task)
// 	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
// 	rec := httptest.NewRecorder()

// 	handler.ServeHTTP(rec, req)
// 	result := rec.Result()
// 	defer result.Body.Close()

// 	if result.StatusCode != http.StatusBadRequest {
// 		t.Fatalf("Invalid JSON and expected status 400 Bad Request, got %d", result.StatusCode)
// 	}

// }

// func TestQueueHandler_IntegrationFailure3(t *testing.T) {
// 	db := TestDB(t)

// 	handler := queueHandler(db)

// 	db.Close()

// 	task := Task{
// 		ID:          "test",
// 		ContainsPHI: true,
// 		Payload: map[string]interface{}{
// 			"patientName": "Angela",
// 			"phone":       "9491203489",
// 		},
// 	}

// 	body, _ := json.Marshal(task)
// 	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
// 	rec := httptest.NewRecorder()

// 	handler.ServeHTTP(rec, req)
// 	result := rec.Result()
// 	defer result.Body.Close()

// 	if result.StatusCode != http.StatusInternalServerError {
// 		t.Fatalf("Expected status 500 Internal Server Error, got %d", result.StatusCode)
// 	}

// }

// func TestQueueHandler_DBFailure(t *testing.T) {
// 	// Set up test DB and then close it to simulate failure
// 	db := TestDB(t)
// 	db.Close() // DB is now closed

// 	handler := queueHandler(db)

// 	task := Task{
// 		ID:          "fail1",
// 		ContainsPHI: true,
// 		Payload: map[string]interface{}{
// 			"patientName": "Charlie",
// 			"phone":       "1234567890",
// 		},
// 	}

// 	body, _ := json.Marshal(task)
// 	req := httptest.NewRequest(http.MethodPost, "/queue", bytes.NewReader(body))
// 	rec := httptest.NewRecorder()

// 	handler.ServeHTTP(rec, req)
// 	result := rec.Result()
// 	defer result.Body.Close()

// 	if result.StatusCode != http.StatusInternalServerError {
// 		t.Fatalf("Expected status 500 Internal Server Error, got %d", result.StatusCode)
// 	}

// 	respBody, _ := io.ReadAll(result.Body)
// 	if !strings.Contains(string(respBody), "Failed to save task") {
// 		t.Errorf("Expected error message about failing to save task, got: %s", string(respBody))
// 	}
// }
