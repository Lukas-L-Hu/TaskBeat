package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.etcd.io/bbolt"
)

var db *bbolt.DB

type Task struct {
	ID          string                 `json:"id"`
	Payload     map[string]interface{} `json:"payload"`
	ContainsPHI bool                   `json:"containsPHI"`
	CreatedAt   time.Time              `json:"createdAt"`
}

func concealPHI(task *Task) {
	// This conceals the protected health information (PHI)
	if !task.ContainsPHI {
		return
	}

	infoKeys := []string{
		"patientName",
		"ssn",
		"dob",
		"address",
		"email",
		"phone",
		"insuranceNumber",
		"medicalRecordNumber",
		"diagnosis",
	}

	for _, key := range infoKeys {
		if _, ok := task.Payload[key].(string); ok {
			task.Payload[key] = "[CONCEALED]"
			log.Printf("TaskBeat: Concealed %s for Task ID %s\n", key, task.ID)
		}
	}
}

func auditLog(task Task) error {
	// This logs each entry and records the time, ID, and if it contains PHI
	if task.ID == "" {
		return fmt.Errorf("Task ID is missing")
	}
	f, err := os.OpenFile("taskbeat_audit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	logEntry := fmt.Sprintf("%s = TaskBeat Queue TaskID=%s PHI=%v\n", time.Now().Format(time.RFC3339), task.ID, task.ContainsPHI)
	_, err = f.WriteString(logEntry)
	return err
}

func queueHandler(db *bbolt.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task Task
		err := json.NewDecoder(r.Body).Decode(&task)
		if err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if task.ID == "" {
			http.Error(w, "Missing task ID", http.StatusBadRequest)
			return
		}

		if len(task.Payload) == 0 {
			http.Error(w, "Payload cannot be empty", http.StatusBadRequest)
			return
		}

		allowedKeys := map[string]bool{
			"patientName":         true,
			"ssn":                 true,
			"dob":                 true,
			"address":             true,
			"email":               true,
			"phone":               true,
			"diagnosis":           true,
			"insuranceNumber":     true,
			"medicalRecordNumber": true,
		}

		for key := range task.Payload {
			if !allowedKeys[key] {
				http.Error(w, fmt.Sprintf("Invalid key %s", key), http.StatusBadRequest)
				return
			}
		}

		if task.CreatedAt.IsZero() {
			task.CreatedAt = time.Now()
		}

		concealPHI(&task)

		if err := auditLog(task); err != nil {
			log.Printf("TaskBeat: Failed audit log: %v\n", err)
		}

		err = db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("Tasks"))
			data, _ := json.Marshal(task)
			return b.Put([]byte(task.ID), data)
		})

		if err != nil {
			http.Error(w, "Failed to save task", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task)
	}
}

func main() {
	// Runs everything
	var err error
	db, err = bbolt.Open("taskbeat.db", 0600, nil)
	if err != nil {
		log.Fatalf("Couldn't open db: %v", err)
	}
	defer db.Close()
	db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Tasks"))
		return err
	})
	http.HandleFunc("/queue", queueHandler(db))
	fmt.Println("TaskBeat running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
