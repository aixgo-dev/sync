package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aixgo-dev/sync/internal/store"
)

// TestCreateAndGet verifies creating a job and fetching it with GET.
func TestCreateAndGet(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := NewServer(memStore)
	handler := server.NewHandler()

	org := "testorg"
	project := "testproj"

	// 1. Create a job
	reqBody := CreateJobRequest{
		Kind:    "code_offload",
		Payload: map[string]interface{}{"repo": "test/repo", "ref": "main"},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	createReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs", org, project), bytes.NewReader(bodyBytes))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", createRec.Code, createRec.Body.String())
	}

	createETag := createRec.Header().Get("ETag")
	if createETag == "" {
		t.Error("expected ETag header to be set, but got empty")
	}

	var createdJob Job
	if err := json.Unmarshal(createRec.Body.Bytes(), &createdJob); err != nil {
		t.Fatalf("failed to unmarshal created job: %v", err)
	}

	if createdJob.ID == "" {
		t.Error("expected job ID to be generated and non-empty")
	}
	if createdJob.Kind != "code_offload" {
		t.Errorf("expected job kind 'code_offload', got %q", createdJob.Kind)
	}
	if createdJob.Status != "queued" {
		t.Errorf("expected job status 'queued', got %q", createdJob.Status)
	}
	if createdJob.OrgID != org {
		t.Errorf("expected org_id %q, got %q", org, createdJob.OrgID)
	}
	if createdJob.ProjectID != project {
		t.Errorf("expected project_id %q, got %q", project, createdJob.ProjectID)
	}
	if createdJob.Type != "job" {
		t.Errorf("expected type 'job', got %q", createdJob.Type)
	}
	if createdJob.CreatedAt == "" || createdJob.UpdatedAt == "" {
		t.Error("expected timestamps to be populated")
	}

	// 2. Fetch the job with GET
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s", org, project, createdJob.ID), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", getRec.Code, getRec.Body.String())
	}

	getETag := getRec.Header().Get("ETag")
	if getETag != createETag {
		t.Errorf("expected GET ETag %q to match POST ETag %q", getETag, createETag)
	}

	var fetchedJob Job
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetchedJob); err != nil {
		t.Fatalf("failed to unmarshal fetched job: %v", err)
	}

	if fetchedJob.ID != createdJob.ID {
		t.Errorf("fetched job ID %q does not match created job ID %q", fetchedJob.ID, createdJob.ID)
	}
	if fetchedJob.Status != "queued" {
		t.Errorf("expected fetched job status 'queued', got %q", fetchedJob.Status)
	}
}

// TestClaimSuccessPath verifies the successful claim flow of a queued job.
func TestClaimSuccessPath(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := NewServer(memStore)
	handler := server.NewHandler()

	org := "testorg"
	project := "testproj"

	// 1. Create a job
	reqBody := CreateJobRequest{Kind: "command"}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs", org, project), bytes.NewReader(bodyBytes))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var createdJob Job
	_ = json.Unmarshal(createRec.Body.Bytes(), &createdJob)

	// 2. Claim the job
	claimReqBody := ClaimJobRequest{WorkerID: "worker-xyz"}
	claimBytes, _ := json.Marshal(claimReqBody)
	claimReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s/claim", org, project, createdJob.ID), bytes.NewReader(claimBytes))
	claimRec := httptest.NewRecorder()
	handler.ServeHTTP(claimRec, claimReq)

	if claimRec.Code != http.StatusOK {
		t.Fatalf("expected claim status 200 OK, got %d. Body: %s", claimRec.Code, claimRec.Body.String())
	}

	var claimedJob Job
	if err := json.Unmarshal(claimRec.Body.Bytes(), &claimedJob); err != nil {
		t.Fatalf("failed to unmarshal claimed job: %v", err)
	}

	if claimedJob.Status != "claimed" {
		t.Errorf("expected claimed job status 'claimed', got %q", claimedJob.Status)
	}
	if claimedJob.AssigneeWorker != "worker-xyz" {
		t.Errorf("expected assignee_worker 'worker-xyz', got %q", claimedJob.AssigneeWorker)
	}

	claimETag := claimRec.Header().Get("ETag")
	if claimETag == "" {
		t.Error("expected claim response to return an updated ETag")
	}

	// 3. GET the job to verify state was stored correctly
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s", org, project, createdJob.ID), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var fetchedJob Job
	_ = json.Unmarshal(getRec.Body.Bytes(), &fetchedJob)

	if fetchedJob.Status != "claimed" {
		t.Errorf("expected fetched job status to be 'claimed', got %q", fetchedJob.Status)
	}
	if fetchedJob.AssigneeWorker != "worker-xyz" {
		t.Errorf("expected fetched job assignee_worker to be 'worker-xyz', got %q", fetchedJob.AssigneeWorker)
	}
	if getRec.Header().Get("ETag") != claimETag {
		t.Errorf("expected GET ETag to match new claim ETag")
	}
}

// TestDoubleClaimFailure verifies that a second claim attempt fails with 409 Conflict.
func TestDoubleClaimFailure(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := NewServer(memStore)
	handler := server.NewHandler()

	org := "testorg"
	project := "testproj"

	// 1. Create a job
	reqBody := CreateJobRequest{Kind: "custom"}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs", org, project), bytes.NewReader(bodyBytes))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var createdJob Job
	_ = json.Unmarshal(createRec.Body.Bytes(), &createdJob)

	// 2. Claim first time (should succeed)
	claim1ReqBody := ClaimJobRequest{WorkerID: "worker-first"}
	claim1Bytes, _ := json.Marshal(claim1ReqBody)
	claim1Req := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s/claim", org, project, createdJob.ID), bytes.NewReader(claim1Bytes))
	claim1Rec := httptest.NewRecorder()
	handler.ServeHTTP(claim1Rec, claim1Req)

	if claim1Rec.Code != http.StatusOK {
		t.Fatalf("first claim expected 200 OK, got %d", claim1Rec.Code)
	}

	// 3. Claim second time (should fail)
	claim2ReqBody := ClaimJobRequest{WorkerID: "worker-second"}
	claim2Bytes, _ := json.Marshal(claim2ReqBody)
	claim2Req := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s/claim", org, project, createdJob.ID), bytes.NewReader(claim2Bytes))
	claim2Rec := httptest.NewRecorder()
	handler.ServeHTTP(claim2Rec, claim2Req)

	if claim2Rec.Code != http.StatusConflict {
		t.Errorf("expected second claim to fail with 409 Conflict, got %d. Body: %s", claim2Rec.Code, claim2Rec.Body.String())
	}

	// 4. Verify first assignee is retained
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s", org, project, createdJob.ID), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var fetchedJob Job
	_ = json.Unmarshal(getRec.Body.Bytes(), &fetchedJob)

	if fetchedJob.Status != "claimed" {
		t.Errorf("expected job status 'claimed', got %q", fetchedJob.Status)
	}
	if fetchedJob.AssigneeWorker != "worker-first" {
		t.Errorf("expected assignee_worker 'worker-first' to be retained, got %q", fetchedJob.AssigneeWorker)
	}
}

// TestParallelClaimRace launches multiple goroutines claiming concurrently and verifies that exactly one succeeds.
func TestParallelClaimRace(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := NewServer(memStore)
	handler := server.NewHandler()

	org := "testorg"
	project := "testproj"

	// 1. Create a job
	reqBody := CreateJobRequest{Kind: "code_offload"}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs", org, project), bytes.NewReader(bodyBytes))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var createdJob Job
	_ = json.Unmarshal(createRec.Body.Bytes(), &createdJob)

	// 2. Spin up 30 goroutines claiming the job concurrently
	numWorkers := 30
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	var successCount int64
	var conflictCount int64
	var preconditionFailedCount int64
	var otherCount int64

	// Track which worker got registered as winner
	var winnerWorker string
	var winnerMu sync.Mutex

	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		go func() {
			defer wg.Done()

			claimReqBody := ClaimJobRequest{WorkerID: workerID}
			claimBytes, _ := json.Marshal(claimReqBody)
			claimReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s/claim", org, project, createdJob.ID), bytes.NewReader(claimBytes))
			claimRec := httptest.NewRecorder()
			handler.ServeHTTP(claimRec, claimReq)

			switch claimRec.Code {
			case http.StatusOK:
				atomic.AddInt64(&successCount, 1)
				winnerMu.Lock()
				winnerWorker = workerID
				winnerMu.Unlock()
			case http.StatusConflict:
				atomic.AddInt64(&conflictCount, 1)
			case http.StatusPreconditionFailed:
				atomic.AddInt64(&preconditionFailedCount, 1)
			default:
				atomic.AddInt64(&otherCount, 1)
			}
		}()
	}

	wg.Wait()

	// 3. Assert exactly one worker succeeded
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful claim, got %d", successCount)
	}

	// All other workers must have failed due to CAS precondition checks or state validation.
	totalFailures := conflictCount + preconditionFailedCount
	if totalFailures != int64(numWorkers-1) {
		t.Errorf("expected %d failures, got %d (Conflicts: %d, PreconditionFailed: %d, Other: %d)",
			numWorkers-1, totalFailures, conflictCount, preconditionFailedCount, otherCount)
	}

	if otherCount > 0 {
		t.Errorf("unexpected status codes encountered: %d responses", otherCount)
	}

	// 4. Fetch the final job to verify winner and retention
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs/%s", org, project, createdJob.ID), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var fetchedJob Job
	_ = json.Unmarshal(getRec.Body.Bytes(), &fetchedJob)

	if fetchedJob.Status != "claimed" {
		t.Errorf("expected final job status to be 'claimed', got %q", fetchedJob.Status)
	}
	if fetchedJob.AssigneeWorker != winnerWorker {
		t.Errorf("expected assignee_worker to be %q, got %q", winnerWorker, fetchedJob.AssigneeWorker)
	}
}

// TestInvalidJobKind verifies validation of job kinds during creation.
func TestInvalidJobKind(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := NewServer(memStore)
	handler := server.NewHandler()

	org := "testorg"
	project := "testproj"

	reqBody := CreateJobRequest{Kind: "invalid_kind"}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/jobs", org, project), bytes.NewReader(bodyBytes))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", createRec.Code)
	}
}
