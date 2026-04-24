package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
)

type NotificationConfig struct {
	Emails   []string `json:"emails"`
	Webhooks []string `json:"webhooks"`
}

func loadNotificationConfig(path string) (*NotificationConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read notification config: %w", err)
	}
	var cfg NotificationConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse notification config: %w", err)
	}
	return &cfg, nil
}

type NotificationSubscriber struct {
	cfg         NotificationConfig
	sesClient   *ses.Client
	fromAddress string
	httpClient  *http.Client
}

func NewNotificationSubscriber(cfg NotificationConfig, sesClient *ses.Client, fromAddress string) *NotificationSubscriber {
	return &NotificationSubscriber{
		cfg:         cfg,
		sesClient:   sesClient,
		fromAddress: fromAddress,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *NotificationSubscriber) OnEvent(e Event) {
	if e.Type != EventWorkflowDone {
		return
	}

	status := "succeeded"
	if e.Err != nil {
		status = "failed"
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	for _, addr := range n.cfg.Emails {
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			if err := n.sendEmail(ctx, a, e.WorkflowName, status, e.Err); err != nil {
				log.Printf("[notify] email to %s failed: %v", a, err)
			}
		}(addr)
	}

	for _, url := range n.cfg.Webhooks {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if err := n.callWebhook(ctx, u, e.WorkflowName, status, e.Err); err != nil {
				log.Printf("[notify] webhook %s failed: %v", u, err)
			}
		}(url)
	}

	wg.Wait()
}

func (n *NotificationSubscriber) sendEmail(ctx context.Context, addr, workflowName, status string, runErr error) error {
	subject := fmt.Sprintf("Workflow %q %s", workflowName, status)
	body := fmt.Sprintf("Workflow %q completed with status: %s", workflowName, status)
	if runErr != nil {
		body += fmt.Sprintf("\nError: %v", runErr)
	}

	_, err := n.sesClient.SendEmail(ctx, &ses.SendEmailInput{
		Source: aws.String(n.fromAddress),
		Destination: &sestypes.Destination{
			ToAddresses: []string{addr},
		},
		Message: &sestypes.Message{
			Subject: &sestypes.Content{Data: aws.String(subject)},
			Body:    &sestypes.Body{Text: &sestypes.Content{Data: aws.String(body)}},
		},
	})
	if err != nil {
		return err
	}
	log.Printf("[notify] ✉  email sent to %s", addr)
	return nil
}

type webhookPayload struct {
	Workflow string `json:"workflow"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

func (n *NotificationSubscriber) callWebhook(ctx context.Context, url, workflowName, status string, runErr error) error {
	p := webhookPayload{Workflow: workflowName, Status: status}
	if runErr != nil {
		p.Error = runErr.Error()
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	log.Printf("[notify] webhook called: %s → %d", url, resp.StatusCode)
	return nil
}
