package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type transactionTestInstance struct {
	check    func(string, bool) error
	shutdown func() error
}

func (i transactionTestInstance) healthy(v string, p bool) error { return i.check(v, p) }
func (i transactionTestInstance) stop() error                    { return i.shutdown() }

func TestOfflineUpdateTransactionFaultRecovery(t *testing.T) {
	for _, stage := range []string{"success", "backup", "install", "probe", "normal", "stop", "recovery"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			work := filepath.Join(root, ".yunkun-local", "updates", "fixture")
			job := offlineJob{OfflineInstall: OfflineInstall{Root: root}, Work: work, From: "v1.1.0-offline", To: "v1.1.1-offline"}
			write := func(rel, value string) {
				t.Helper()
				file := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			read := func(rel string) string {
				t.Helper()
				b, err := os.ReadFile(filepath.Join(root, rel))
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			write("bin/yunkun", "old-program")
			write(".yunkun-local/postgres-data/record", "old-data")
			write("bin/jwt.key", "original-key")
			write("skills/custom/SKILL.md", "user-skill")
			b, _ := json.Marshal(job)
			write(".yunkun-local/updates/fixture/job.json", string(b))
			write(".yunkun-local/updates/fixture/candidate/bin/yunkun", "new-program")
			write(".yunkun-local/updates/fixture/candidate/skills/custom/SKILL.md", "new-default")
			path := filepath.Join(work, "job.json")
			lastPhase := ""
			stopped, restored := true, false
			txn := offlineTransaction{job: job, state: func(phase, message string, err error) error { lastPhase = phase; return nil }}
			txn.files = func(op string) error {
				if op == stage {
					if op == "install" {
						write("bin/yunkun", "partial")
					}
					return errors.New("injected " + stage)
				}
				if op == "restore" {
					if !stopped {
						t.Fatal("restore attempted before process stopped")
					}
					restored = true
				}
				return OfflineFiles(path, op)
			}
			txn.launch = func(probe bool) (offlineInstance, error) {
				program := read("bin/yunkun")
				stopped = false
				return transactionTestInstance{
					check: func(version string, expectedProbe bool) error {
						if probe != expectedProbe {
							t.Fatal("incorrect startup mode")
						}
						if program == "new-program" {
							if version != job.To {
								t.Fatal("new program checked as old")
							}
							write(".yunkun-local/postgres-data/record", "migrated-data")
							if (probe && (stage == "probe" || stage == "recovery")) || (!probe && stage == "normal") {
								return errors.New("injected startup failure")
							}
						} else if program != "old-program" || version != job.From {
							t.Fatal("recovery started mismatched program")
						}
						if stage == "recovery" && program == "old-program" {
							return errors.New("injected recovery failure")
						}
						return nil
					},
					shutdown: func() error {
						if stage == "stop" {
							return errors.New("injected stop failure")
						}
						stopped = true
						return nil
					},
				}, nil
			}
			err := txn.run()
			if (err == nil) != (stage == "success") {
				t.Fatalf("result: %v", err)
			}
			switch stage {
			case "success":
				if lastPhase != "completed" || read("bin/yunkun") != "new-program" || read(".yunkun-local/postgres-data/record") != "migrated-data" {
					t.Fatal("success state incomplete")
				}
			case "stop", "recovery":
				if lastPhase != "recovery_required" {
					t.Fatal("unsafe recovery was not stopped")
				}
				if stage == "stop" && restored {
					t.Fatal("copied a running database")
				}
			default:
				want := "rolled_back"
				if stage == "backup" {
					want = "failed"
				}
				if lastPhase != want || read("bin/yunkun") != "old-program" || read(".yunkun-local/postgres-data/record") != "old-data" {
					t.Fatal("matching program and data not recovered")
				}
			}
			if read("bin/jwt.key") != "original-key" || read("skills/custom/SKILL.md") != "user-skill" {
				t.Fatal("user key or Skill was changed")
			}
		})
	}
}
