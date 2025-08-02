package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Task struct {
	ID          string                 `json:"id"`
	Payload     map[string]interface{} `json:"payload"`
	ContainsPHI bool                   `json:"containsPHI"`
	CreatedAt   time.Time              `json:"createdAt"`
}

func concealPHI(task *Task) {
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
	f, err := os.OpenFile("taskbeat_audit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	logEntry := fmt.Sprintf("%s = TaskBeat Queue TaskID=%s PHI=%v\n", time.Now().Format(time.RFC3339), task.ID, task.ContainsPHI)
	_, err = f.WriteString(logEntry)
	return err
}

func queueHandler(w http.ResponseWriter, r *http.Request) {
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
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	concealPHI(&task)

	if err := auditLog(task); err != nil {
		log.Printf("TaskBeat: Failed audit log: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func main() {
	http.HandleFunc("/queue", queueHandler)
	fmt.Println("TaskBeat running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
