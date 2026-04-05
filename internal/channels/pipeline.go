package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrChannelDisabled = errors.New("channel is disabled")
	ErrDeduped         = errors.New("channel event deduplicated")
)

type WorkflowEngine interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
}

type AttachmentService interface {
	Store(ctx context.Context, tenantID, caseID uuid.UUID, in AttachmentInput) (AttachmentRef, error)
}

type Pipeline struct {
	workflowEngine WorkflowEngine
	channelStore   ChannelStore
	dedupChecker   *DedupChecker
	adapterEngine  *AdapterEngine
	attachments    AttachmentService
}

func NewPipeline(workflowEngine WorkflowEngine, channelStore ChannelStore, attachments AttachmentService) *Pipeline {
	return &Pipeline{
		workflowEngine: workflowEngine,
		channelStore:   channelStore,
		dedupChecker:   NewDedupChecker(),
		adapterEngine:  NewAdapterEngine(),
		attachments:    attachments,
	}
}

func (p *Pipeline) Process(ctx context.Context, req PipelineRequest) (PipelineResult, error) {
	start := time.Now()
	if req.TenantID == uuid.Nil || req.ChannelID == uuid.Nil {
		return PipelineResult{}, fmt.Errorf("tenant_id and channel_id are required")
	}
	rawPayload := truncateRawPayload(req.Data)
	result := PipelineResult{}
	processingErr := p.channelStore.WithTx(ctx, func(txCtx context.Context, tx TxStore) error {
		ch, err := tx.GetChannel(txCtx, req.TenantID, req.ChannelID)
		if err != nil {
			return err
		}
		if !ch.Enabled {
			return ErrChannelDisabled
		}

		dedup := ch.DedupConfig
		if req.DedupOverride != nil {
			dedup = *req.DedupOverride
		}
		isDup, matchCaseID, err := p.dedupChecker.Check(txCtx, tx, req.TenantID, ch.CaseTypeID, ch.ID, dedup, req.Data)
		if err != nil {
			return err
		}
		if isDup {
			eventID, err := tx.RecordEvent(txCtx, &ChannelEvent{
				TenantID:     req.TenantID,
				ChannelID:    ch.ID,
				RawPayload:   rawPayload,
				Status:       EventDeduped,
				CaseID:       matchCaseID,
				ProcessingMS: int(time.Since(start).Milliseconds()),
			})
			if err != nil {
				return err
			}
			if matchCaseID != nil {
				result.CaseID = *matchCaseID
			}
			result.Deduped = true
			result.EventID = eventID
			return ErrDeduped
		}

		adaptedRaw, err := p.adapterEngine.Apply(ch.AdapterConfig, req.Data)
		if err != nil {
			return err
		}
		adapted := map[string]any{}
		if err := json.Unmarshal(adaptedRaw, &adapted); err != nil {
			return fmt.Errorf("decode adapted payload: %w", err)
		}

		var caseID uuid.UUID
		createdCase := true
		if matchCaseID != nil {
			caseID = *matchCaseID
			createdCase = false
			if err := tx.UpdateCaseData(txCtx, req.TenantID, caseID, adapted); err != nil {
				return err
			}
		} else {
			caseID, err = tx.CreateCase(txCtx, CreateOrUpdateCaseInput{
				TenantID:   req.TenantID,
				ChannelID:  ch.ID,
				CaseTypeID: ch.CaseTypeID,
				WorkflowID: ch.WorkflowID,
				Data:       adapted,
				ActorID:    req.ActorID,
			})
			if err != nil {
				return err
			}
		}

		refs := make([]AttachmentRef, 0, len(req.Attachments))
		if p.attachments != nil {
			for _, in := range req.Attachments {
				ref, err := p.attachments.Store(txCtx, req.TenantID, caseID, in)
				if err != nil {
					return err
				}
				refs = append(refs, ref)
			}
		}
		if len(refs) > 0 {
			if err := tx.UpdateCaseData(txCtx, req.TenantID, caseID, map[string]any{"attachments": refs}); err != nil {
				return err
			}
		}

		if createdCase && ch.WorkflowID != nil && p.workflowEngine != nil {
			if err := p.workflowEngine.EvaluateDAG(txCtx, caseID); err != nil {
				return err
			}
		}

		eventID, err := tx.RecordEvent(txCtx, &ChannelEvent{
			TenantID:     req.TenantID,
			ChannelID:    ch.ID,
			RawPayload:   rawPayload,
			Attachments:  refs,
			CaseID:       &caseID,
			Status:       EventProcessed,
			ProcessingMS: int(time.Since(start).Milliseconds()),
		})
		if err != nil {
			return err
		}
		result.CaseID = caseID
		result.EventID = eventID
		return nil
	})

	if processingErr == nil {
		return result, nil
	}
	if errors.Is(processingErr, ErrDeduped) {
		return result, ErrDeduped
	}

	_, _ = p.channelStore.RecordFailedEvent(ctx, &ChannelEvent{
		TenantID:     req.TenantID,
		ChannelID:    req.ChannelID,
		RawPayload:   rawPayload,
		Status:       EventFailed,
		ErrorMessage: processingErr.Error(),
		ProcessingMS: int(time.Since(start).Milliseconds()),
	})
	return PipelineResult{}, processingErr
}

func truncateRawPayload(raw []byte) []byte {
	if len(raw) <= 1024*1024 {
		if len(raw) == 0 {
			return []byte(`{}`)
		}
		return raw
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []byte(`{"_truncated":true}`)
	}
	payload["_truncated"] = true
	payload["_original_size"] = len(raw)
	out, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"_truncated":true}`)
	}
	if len(out) > 1024*1024 {
		return []byte(`{"_truncated":true}`)
	}
	return out
}
