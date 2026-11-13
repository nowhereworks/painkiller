package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Exam folders are trusted server-side content, never paths supplied by clients.
type examDefinition struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Minutes     int      `json:"minutes"`
	Lab         *labSpec `json:"lab,omitempty"`
	dir         string
	tasks       []task
}

func loadExams(root string) ([]examDefinition, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var exams []examDefinition
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		var exam examDefinition
		b, err := os.ReadFile(filepath.Join(dir, "test.json"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		if err = json.Unmarshal(b, &exam); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		if !regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]{3}$`).MatchString(exam.ID) ||
			exam.ID != entry.Name() || exam.Title == "" || exam.Minutes < 1 || exam.Minutes > 1440 {
			return nil, fmt.Errorf("%s: invalid test metadata (ID must match folder, e.g. CKA-001)", dir)
		}
		for _, name := range []string{"setup.yaml", "reference-solutions.sh"} {
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", dir, err)
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%s: %s must be a regular file", dir, name)
			}
		}
		b, err = os.ReadFile(filepath.Join(dir, "tasks.json"))
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &exam.tasks); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		if err = validateTasks(exam.tasks); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		exam.dir = dir
		if err := validateLabSpec(exam); err != nil {
			return nil, err
		}
		exams = append(exams, exam)
	}
	if len(exams) == 0 {
		return nil, fmt.Errorf("%s: no tests found", root)
	}
	return exams, nil
}

func (a *app) selectExam(id string) error {
	for _, exam := range a.exams {
		if exam.ID == id {
			minutes := exam.Minutes
			if override := os.Getenv("PAINKILLER_MINUTES"); override != "" {
				var err error
				minutes, err = strconv.Atoi(override)
				if err != nil || minutes < 1 || minutes > 1440 {
					return fmt.Errorf("PAINKILLER_MINUTES must be between 1 and 1440")
				}
			}
			a.tasks = exam.tasks
			a.duration = time.Duration(minutes) * time.Minute
			return nil
		}
	}
	return fmt.Errorf("unknown test %q", id)
}

func (a *app) setupPath() string {
	for _, exam := range a.exams {
		if exam.ID == a.state.TestID {
			return filepath.Join(exam.dir, "setup.yaml")
		}
	}
	return ""
}

func (a *app) preparationPath() (string, error) {
	for _, exam := range a.exams {
		if exam.ID == a.state.TestID && exam.Lab != nil && exam.Lab.Prepare != "" {
			return trustedExamFile(exam, exam.Lab.Prepare)
		}
	}
	return "", nil
}

// Restore definitions before score migration or resuming background grading.
func (a *app) restoreExam() (bool, error) {
	migrated := a.state.TestID == "" && a.state.Phase != "idle"
	if migrated {
		a.state.TestID = "CKA-001"
	}
	if a.state.TestID != "" {
		if err := a.selectExam(a.state.TestID); err != nil {
			return false, err
		}
		if a.state.Lab != nil && a.state.Lab.TestID != a.state.TestID {
			return false, fmt.Errorf("saved lab belongs to %s, not selected test %s; refusing to change topology", a.state.Lab.TestID, a.state.TestID)
		}
	}
	return migrated, nil
}

func (a *app) testList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// Do not expose task solutions or capture/check commands in the catalog.
	type topology struct {
		Controllers int `json:"controllers"`
		Workers     int `json:"workers"`
	}
	type catalogExam struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Minutes     int       `json:"minutes"`
		Lab         *topology `json:"lab,omitempty"`
	}
	public := make([]catalogExam, 0, len(a.exams))
	for _, exam := range a.exams {
		entry := catalogExam{ID: exam.ID, Title: exam.Title, Description: exam.Description, Minutes: exam.Minutes}
		if exam.Lab != nil {
			entry.Lab = &topology{Controllers: exam.Lab.Controllers, Workers: exam.Lab.Workers}
		}
		public = append(public, entry)
	}
	if minutes, err := strconv.Atoi(os.Getenv("PAINKILLER_MINUTES")); err == nil && minutes >= 1 && minutes <= 1440 {
		for i := range public {
			public[i].Minutes = minutes
		}
	}
	_ = json.NewEncoder(w).Encode(public)
}
