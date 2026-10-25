package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"yunkun/console/notify"
)

const (
	FindingQualityPass          = "PASS"
	FindingQualityNeedsMore     = "NEEDS_MORE"
	FindingQualityDowngrade     = "DOWNGRADE"
	FindingQualityDuplicateRisk = "DUPLICATE_RISK"
	FindingQualityBlocked       = "BLOCKED"
)

type FindingQuality struct {
	FindingID             int64      `json:"finding_id"`
	QualityStatus         string     `json:"quality_status"`
	VulnerabilityType     string     `json:"vulnerability_type"`
	VerificationType      string     `json:"verification_type"`
	BaselineEvidence      string     `json:"baseline_evidence"`
	ProofEvidence         string     `json:"proof_evidence"`
	VerificationEvidence  string     `json:"verification_evidence"`
	VerificationRationale string     `json:"verification_rationale"`
	ReproductionCount     int        `json:"reproduction_count"`
	SuccessCount          int        `json:"success_count"`
	BlockingReason        string     `json:"blocking_reason"`
	DuplicateRisk         string     `json:"duplicate_risk"`
	EvidenceIntegrity     string     `json:"evidence_integrity"`
	Reviewer              string     `json:"reviewer"`
	ReviewedAt            *time.Time `json:"reviewed_at,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func ValidFindingQualityStatus(status string) bool {
	switch status {
	case FindingQualityPass, FindingQualityNeedsMore, FindingQualityDowngrade,
		FindingQualityDuplicateRisk, FindingQualityBlocked:
		return true
	default:
		return false
	}
}

func FindingDeliveryMetadataBlockers(findingID int64, severity, status string) []string {
	var blockers []string
	if severity == SeverityLow {
		blockers = append(blockers, fmt.Sprintf("#%d:LOW_SEVERITY", findingID))
	}
	if status != FindingConfirmed && status != FindingResolved &&
		status != FindingFixed && status != FindingRiskAccepted {
		blockers = append(blockers, fmt.Sprintf("#%d:TRIAGE_%s", findingID, strings.ToUpper(status)))
	}
	return blockers
}

// ExcludedFindingClass identifies labels that the SRC workflow treats as
// low-value standalone leads. They may still be mentioned as part of a proven
// impact chain, but must not be submitted as an independent formal finding.
func ExcludedFindingClass(values ...string) string {
	var words []string
	var rawValues []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		rawValues = append(rawValues, value)
		var b strings.Builder
		for _, r := range value {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else {
				b.WriteByte(' ')
			}
		}
		words = append(words, strings.Fields(b.String())...)
	}
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	switch {
	case set["cors"]:
		return "CORS"
	case set["idor"]:
		return "IDOR"
	case set["bola"]:
		return "BOLA"
	}
	joined := strings.Join(words, " ")
	switch {
	case strings.Contains(joined, "cross origin resource sharing"):
		return "CORS"
	case strings.Contains(joined, "insecure direct object reference"):
		return "IDOR"
	case strings.Contains(joined, "broken object level authorization"):
		return "BOLA"
	}
	raw := strings.Join(rawValues, " ")
	switch {
	case strings.Contains(raw, "跨域资源共享"):
		return "CORS"
	case strings.Contains(raw, "不安全直接对象引用"), strings.Contains(raw, "对象引用越权"):
		return "IDOR"
	case strings.Contains(raw, "对象级别授权失效"), strings.Contains(raw, "对象级授权失效"):
		return "BOLA"
	default:
		return ""
	}
}

func evaluateFindingQuality(in RecordFindingInput, traffic []PreparedTrafficEvidence) FindingQuality {
	q := FindingQuality{
		QualityStatus:     FindingQualityNeedsMore,
		VulnerabilityType: strings.TrimSpace(in.VulnClass),
		VerificationType:  "type_specific",
	}
	if class := ExcludedFindingClass(in.VulnClass, in.Name); class != "" {
		q.QualityStatus = FindingQualityBlocked
		q.BlockingReason = fmt.Sprintf("%s 不能作为独立正式漏洞；请用 record_fact 保留线索，只有已证实的数据读取、权限提升或重大业务影响链才可报告", class)
		q.EvidenceIntegrity = "not_applicable"
		return q
	}
	if strings.TrimSpace(in.Summary) == "" || strings.TrimSpace(in.Evidence) == "" {
		q.BlockingReason = "缺少漏洞摘要或可验证证据"
		q.EvidenceIntegrity = "incomplete"
		return q
	}
	q.ProofEvidence = strings.TrimSpace(in.Evidence)
	q.EvidenceIntegrity = "text_evidence"
	for _, item := range traffic {
		switch item.Ref.Role {
		case "baseline":
			q.BaselineEvidence = item.Snapshot.ID
		case "proof":
			q.ProofEvidence = item.Snapshot.ID
		case "verification":
			q.VerificationEvidence = item.Snapshot.ID
		}
	}
	if len(traffic) > 0 {
		q.EvidenceIntegrity = "captured_http_evidence"
	}
	if in.Severity == SeverityLow {
		q.QualityStatus = FindingQualityDowngrade
		q.BlockingReason = "低危记录不进入中危及以上正式报告"
		return q
	}
	if in.Severity != SeverityCritical && in.Severity != SeverityHigh && in.Severity != SeverityMedium {
		q.BlockingReason = "严重等级不在正式报告允许范围内"
		return q
	}
	return q
}

func insertFindingQualityTx(tx *sql.Tx, findingID int64, q FindingQuality) error {
	_, err := tx.Exec(`INSERT INTO finding_quality
	(finding_id, quality_status, vulnerability_type, verification_type,
	 baseline_evidence, proof_evidence, verification_evidence, verification_rationale,
	 reproduction_count, success_count, blocking_reason, duplicate_risk,
	 evidence_integrity)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (finding_id) DO NOTHING`,
		findingID, q.QualityStatus, q.VulnerabilityType, q.VerificationType,
		q.BaselineEvidence, q.ProofEvidence, q.VerificationEvidence, q.VerificationRationale,
		q.ReproductionCount, q.SuccessCount, q.BlockingReason, q.DuplicateRisk,
		q.EvidenceIntegrity)
	return err
}

func (d *DB) ensureFindingQualityRows(findingIDs []int64) error {
	if len(findingIDs) == 0 {
		return nil
	}
	_, err := d.Exec(`INSERT INTO finding_quality
	(finding_id, quality_status, vulnerability_type, proof_evidence,
	 blocking_reason, evidence_integrity)
	SELECT f.id,
		CASE WHEN f.severity=$2 THEN 'DOWNGRADE' ELSE 'NEEDS_MORE' END,
		COALESCE(f.vulnclass,''), COALESCE(f.evidence,''),
		CASE WHEN f.severity=$2 THEN '低危记录不进入中危及以上正式报告'
		     ELSE '历史记录需要人工审核' END,
		CASE WHEN COALESCE(f.evidence,'')='' THEN 'incomplete' ELSE 'text_evidence' END
	FROM findings f
	WHERE f.id = ANY($1::bigint[])
	ON CONFLICT (finding_id) DO NOTHING`, findingIDs, SeverityLow)
	return err
}

func (d *DB) GetFindingQuality(findingID int64) (*FindingQuality, error) {
	if err := d.ensureFindingQualityRows([]int64{findingID}); err != nil {
		return nil, err
	}
	q := new(FindingQuality)
	err := d.QueryRow(`SELECT finding_id, quality_status, vulnerability_type,
		verification_type, baseline_evidence, proof_evidence, verification_evidence, verification_rationale,
		reproduction_count, success_count, blocking_reason, duplicate_risk,
		evidence_integrity, reviewer, reviewed_at, updated_at
		FROM finding_quality WHERE finding_id=$1`, findingID).Scan(
		&q.FindingID, &q.QualityStatus, &q.VulnerabilityType, &q.VerificationType,
		&q.BaselineEvidence, &q.ProofEvidence, &q.VerificationEvidence, &q.VerificationRationale,
		&q.ReproductionCount, &q.SuccessCount, &q.BlockingReason, &q.DuplicateRisk,
		&q.EvidenceIntegrity, &q.Reviewer, &q.ReviewedAt, &q.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (d *DB) ListFindingQuality(findingIDs []int64) (map[int64]*FindingQuality, error) {
	out := make(map[int64]*FindingQuality)
	if len(findingIDs) == 0 {
		return out, nil
	}
	if err := d.ensureFindingQualityRows(findingIDs); err != nil {
		return nil, err
	}
	rows, err := d.Query(`SELECT finding_id, quality_status, vulnerability_type,
		verification_type, baseline_evidence, proof_evidence, verification_evidence, verification_rationale,
		reproduction_count, success_count, blocking_reason, duplicate_risk,
		evidence_integrity, reviewer, reviewed_at, updated_at
		FROM finding_quality WHERE finding_id = ANY($1::bigint[])`, findingIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		q := new(FindingQuality)
		if err := rows.Scan(&q.FindingID, &q.QualityStatus, &q.VulnerabilityType,
			&q.VerificationType, &q.BaselineEvidence, &q.ProofEvidence,
			&q.VerificationEvidence, &q.VerificationRationale, &q.ReproductionCount, &q.SuccessCount,
			&q.BlockingReason, &q.DuplicateRisk, &q.EvidenceIntegrity,
			&q.Reviewer, &q.ReviewedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		out[q.FindingID] = q
	}
	return out, rows.Err()
}

func (d *DB) SetFindingQuality(ctx context.Context, q FindingQuality) error {
	if q.FindingID <= 0 || !ValidFindingQualityStatus(q.QualityStatus) {
		return errors.New("invalid finding quality")
	}
	q.Reviewer = strings.TrimSpace(q.Reviewer)
	if q.QualityStatus == FindingQualityPass && q.Reviewer == "" {
		return errors.New("PASS quality requires reviewer")
	}
	if q.QualityStatus == FindingQualityPass {
		if strings.TrimSpace(q.VulnerabilityType) == "" || strings.TrimSpace(q.VerificationType) == "" ||
			strings.TrimSpace(q.ProofEvidence) == "" || strings.TrimSpace(q.VerificationEvidence) == "" ||
			(q.ReproductionCount < 3 || q.ReproductionCount > 5) || q.SuccessCount < 1 ||
			q.SuccessCount > q.ReproductionCount || strings.TrimSpace(q.EvidenceIntegrity) != "verified" ||
			(strings.TrimSpace(q.BaselineEvidence) == "" && strings.TrimSpace(q.VerificationRationale) == "") {
			return errors.New("PASS requires type, proof, verification, 3-5 reproductions, success count, verified integrity, and baseline or rationale")
		}
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Lock the finding before its quality row, matching evidence mutations.
	var (
		vulnclass, name, severity, summary string
		taskID                             sql.NullInt64
		assetIDs                           []byte
	)
	if err := tx.QueryRowContext(ctx, `SELECT vulnclass,name,severity,summary,task_id,asset_ids
FROM findings WHERE id=$1 FOR UPDATE`, q.FindingID).Scan(&vulnclass, &name, &severity, &summary, &taskID, &assetIDs); err != nil {
		return err
	}
	if q.QualityStatus == FindingQualityPass {
		if class := ExcludedFindingClass(vulnclass, name, q.VulnerabilityType); class != "" {
			return fmt.Errorf("%s 不能作为独立正式漏洞通过质量审核", class)
		}
		if severity == SeverityLow {
			return errors.New("low severity cannot pass the formal report quality gate")
		}
	}
	var previous string
	if err := tx.QueryRowContext(ctx, `SELECT quality_status FROM finding_quality WHERE finding_id=$1 FOR UPDATE`, q.FindingID).Scan(&previous); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE finding_quality SET quality_status=$2,
		vulnerability_type=$3, verification_type=$4, baseline_evidence=$5,
		proof_evidence=$6, verification_evidence=$7, verification_rationale=$8,
		reproduction_count=$9, success_count=$10, blocking_reason=$11, duplicate_risk=$12,
		evidence_integrity=$13, reviewer=$14,
		reviewed_at=CASE WHEN $14<>'' THEN now() ELSE reviewed_at END,
		updated_at=now() WHERE finding_id=$1`,
		q.FindingID, q.QualityStatus, q.VulnerabilityType, q.VerificationType,
		q.BaselineEvidence, q.ProofEvidence, q.VerificationEvidence, q.VerificationRationale,
		q.ReproductionCount, q.SuccessCount, q.BlockingReason, q.DuplicateRisk,
		q.EvidenceIntegrity, q.Reviewer)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("finding quality not found")
	}
	if q.QualityStatus == FindingQualityPass && previous != FindingQualityPass {
		var assets []int64
		_ = json.Unmarshal(assetIDs, &assets)
		snapshot, err := json.Marshal(notify.Snapshot{
			Kind: notify.EventFindingVerified, FindingID: q.FindingID, TaskID: taskID.Int64,
			VulnClass: vulnclass, Name: name, Severity: severity, Summary: summary, AssetIDs: assets,
		})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_events(kind,finding_id,snapshot)
VALUES($1,$2,$3) ON CONFLICT (finding_id) WHERE kind='finding_verified' DO NOTHING`,
			notify.EventFindingVerified, q.FindingID, string(snapshot)); err != nil {
			return fmt.Errorf("登记已验证漏洞通知失败: %w", err)
		}
	}
	return tx.Commit()
}

func (d *DB) FormalFindingExportBlockers(findings []*DBFinding) ([]string, error) {
	ids := make([]int64, 0, len(findings))
	for _, f := range findings {
		if f != nil {
			ids = append(ids, f.ID)
		}
	}
	quality, err := d.ListFindingQuality(ids)
	if err != nil {
		return nil, err
	}
	var blockers []string
	for _, f := range findings {
		if f == nil {
			continue
		}
		blockers = append(blockers, FindingDeliveryMetadataBlockers(f.ID, f.Severity, f.Status)...)
		if class := ExcludedFindingClass(f.VulnClass, f.Name); class != "" {
			blockers = append(blockers, fmt.Sprintf("#%d:EXCLUDED_%s", f.ID, class))
		}
		q := quality[f.ID]
		if q == nil {
			blockers = append(blockers, fmt.Sprintf("#%d:UNREVIEWED", f.ID))
		} else if q.QualityStatus != FindingQualityPass {
			blockers = append(blockers, fmt.Sprintf("#%d:%s", f.ID, q.QualityStatus))
		}
		if q != nil {
			if class := ExcludedFindingClass(q.VulnerabilityType); class != "" {
				blockers = append(blockers, fmt.Sprintf("#%d:EXCLUDED_%s", f.ID, class))
			}
		}
		if strings.TrimSpace(f.Report) == "" {
			blockers = append(blockers, fmt.Sprintf("#%d:REPORT_MISSING", f.ID))
		}
		if f.ReportEvidenceVersion != f.EvidenceVersion {
			blockers = append(blockers, fmt.Sprintf("#%d:REPORT_EVIDENCE_STALE", f.ID))
		}
	}
	return blockers, nil
}
