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
var taskQueue chan Task

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
		if val, exists := task.Payload[key]; exists {
			if _, isStr := val.(string); isStr {
				task.Payload[key] = "[CONCEALED]"
				log.Printf("TaskBeat: Concealed %s for Task ID %s\n", key, task.ID)
			}
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

func validateTask(task Task) error {
	if task.ID == "" {
		return fmt.Errorf("missing task ID")
	}
	if len(task.Payload) == 0 {
		return fmt.Errorf("payload cannot be empty")
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
			return fmt.Errorf("invalid key %s", key)
		}
	}
	return nil
}

func saveTask(db *bbolt.DB, task Task) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("Tasks"))
		data, _ := json.Marshal(task)
		return b.Put([]byte(task.ID), data)
	})
}

func worker(db *bbolt.DB, tasks <-chan Task) {
	for task := range tasks {
		concealPHI(&task)
		if err := auditLog(task); err != nil {
			log.Printf("TaskBeat: Failed audit log: %v\n", err)
		}
		if err := saveTask(db, task); err != nil {
			log.Printf("TaskBeat: Failed to save Task ID %s: %v", task.ID, err)
		}
	}
}

func queueHandler(db *bbolt.DB, taskQueue chan Task) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := validateTask(task); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if task.CreatedAt.IsZero() {
			task.CreatedAt = time.Now()
		}

		if task.ContainsPHI {
			concealPHI(&task)
		}

		taskQueue <- task

		if err := db.Update(func(tx *bbolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists([]byte("Tasks"))
			if err != nil {
				return err
			}
			data, err := json.Marshal(task)
			if err != nil {
				return err
			}
			return b.Put([]byte(task.ID), data)
		}); err != nil {
			http.Error(w, "Failed to store task in DB", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "task queued"})
	}
}

func main() {
	// Runs everything
	db, err := bbolt.Open("taskbeat.db", 0600, nil)
	if err != nil {
		log.Fatalf("Couldn't open db: %v", err)
	}
	defer db.Close()
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Tasks"))
		return err
	}); err != nil {
		log.Fatalf("Couldn't create bucket: %v", err)
	}

	taskQueue = make(chan Task, 100)
	go worker(db, taskQueue)
	http.HandleFunc("/queue", queueHandler(db, taskQueue))
	fmt.Println("TaskBeat running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
