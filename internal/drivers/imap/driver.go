package imap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	imapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct {
	client *imapclient.Client
}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "imap" }
func (d *Driver) DisplayName() string { return "IMAP" }

func (d *Driver) Connect(ctx context.Context, config drivers.IMAPConfig) error {
	_ = ctx
	if config.Host == "" || config.Port == 0 {
		return fmt.Errorf("imap host and port are required")
	}
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	var (
		c   *imapclient.Client
		err error
	)
	if config.TLS {
		c, err = imapclient.DialTLS(addr, nil)
	} else {
		c, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		return fmt.Errorf("connect imap: %w", err)
	}
	if err := c.Login(config.Username, config.Password).Wait(); err != nil {
		_ = c.Close()
		return fmt.Errorf("imap login: %w", err)
	}
	d.client = c
	return nil
}

func (d *Driver) ListMailboxes(ctx context.Context) ([]string, error) {
	_ = ctx
	if d.client == nil {
		return nil, fmt.Errorf("imap not connected")
	}
	cmd := d.client.List("", "*", nil)
	boxes := []string{}
	for {
		mb := cmd.Next()
		if mb == nil {
			break
		}
		boxes = append(boxes, mb.Mailbox)
	}
	if err := cmd.Close(); err != nil {
		return nil, fmt.Errorf("imap list mailboxes: %w", err)
	}
	sort.Strings(boxes)
	return boxes, nil
}

func (d *Driver) Fetch(ctx context.Context, mailbox string, limit int) ([]drivers.IMAPMessage, error) {
	_ = ctx
	if d.client == nil {
		return nil, fmt.Errorf("imap not connected")
	}
	if limit <= 0 {
		limit = 20
	}
	selectCmd := d.client.Select(mailbox, nil)
	mbox, err := selectCmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("imap select mailbox: %w", err)
	}
	if mbox.NumMessages == 0 {
		return []drivers.IMAPMessage{}, nil
	}
	start := uint32(1)
	if mbox.NumMessages > uint32(limit) {
		start = mbox.NumMessages - uint32(limit) + 1
	}
	seqSet := imap.SeqSetNum(start, mbox.NumMessages)
	bodySection := &imap.FetchItemBodySection{}
	fetchCmd := d.client.Fetch(seqSet, &imap.FetchOptions{Envelope: true, UID: true, BodySection: []*imap.FetchItemBodySection{bodySection}})
	out := make([]drivers.IMAPMessage, 0, limit)
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			return nil, fmt.Errorf("collect fetched message: %w", err)
		}
		im := drivers.IMAPMessage{UID: strconv.FormatUint(uint64(buf.UID), 10), RawHeader: map[string]string{}}
		if buf.Envelope != nil {
			im.Subject = buf.Envelope.Subject
			im.Date = buf.Envelope.Date
			if len(buf.Envelope.From) > 0 {
				im.From = buf.Envelope.From[0].Addr()
			}
		}
		if sec := buf.FindBodySection(bodySection); sec != nil {
			raw, _ := io.ReadAll(bytes.NewReader(sec))
			im.BodyText = string(raw)
		}
		out = append(out, im)
	}
	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	return out, nil
}

func (d *Driver) MarkRead(ctx context.Context, mailbox, uid string) error {
	_ = ctx
	if d.client == nil {
		return fmt.Errorf("imap not connected")
	}
	u, err := strconv.ParseUint(strings.TrimSpace(uid), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid uid: %w", err)
	}
	if _, err := d.client.Select(mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("imap select mailbox: %w", err)
	}
	seq := imap.UIDSetNum(imap.UID(u))
	store := d.client.Store(seq, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil)
	if err := store.Close(); err != nil {
		return fmt.Errorf("imap mark read: %w", err)
	}
	return nil
}

func (d *Driver) Delete(ctx context.Context, mailbox, uid string) error {
	_ = ctx
	if d.client == nil {
		return fmt.Errorf("imap not connected")
	}
	u, err := strconv.ParseUint(strings.TrimSpace(uid), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid uid: %w", err)
	}
	if _, err := d.client.Select(mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("imap select mailbox: %w", err)
	}
	seq := imap.UIDSetNum(imap.UID(u))
	store := d.client.Store(seq, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil)
	if err := store.Close(); err != nil {
		return fmt.Errorf("imap mark delete: %w", err)
	}
	if err := d.client.Expunge().Close(); err != nil {
		return fmt.Errorf("imap expunge: %w", err)
	}
	return nil
}

func (d *Driver) Close() error {
	if d.client == nil {
		return nil
	}
	_ = d.client.Logout().Wait()
	return d.client.Close()
}
