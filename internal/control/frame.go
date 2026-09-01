package control

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Unix sockets and Windows named pipes deliver a continuous stream of bytes, not separate messages.
// The parent and bridge therefore need shared rules for where one message ends and the next begins.
// This file defines those rules:
// Each JSON message starts with four bytes that tell the receiver exactly how
// many bytes belong to the message that follows.
// Both sides read and write this number in the same order, so they can reliably separate one message from the next.

// MaxFrameSize bounds memory allocated for an untrusted control message.
const MaxFrameSize = 1 << 20

// readMessage reads one complete framed message and decodes its JSON payload.
// It rejects unknown fields and extra JSON values so both sides continue to
// follow the same control-message contract.
func readMessage(ctx context.Context, reader io.Reader) (message, error) {
	frame, err := readFrame(ctx, reader)
	if err != nil {
		return message{}, err
	}
	var decoded message
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return message{}, fmt.Errorf("decode control message: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return message{}, err
	}
	if decoded.Type == "" {
		return message{}, errors.New("control message type is required")
	}
	return decoded, nil
}

// ensureEOF confirms that the payload contained exactly one JSON value.
func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("control message contains multiple JSON values")
		}
		return fmt.Errorf("decode control message: %w", err)
	}
	return nil
}

// writeMessage encodes one control message as JSON and sends it as one framed
// message, so the receiver can find its boundary in the byte stream.
func writeMessage(ctx context.Context, writer io.Writer, message message) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode control message: %w", err)
	}
	return writeFrame(ctx, writer, payload)
}

// readFrame reads the message length first, then reads exactly that many payload bytes.
// It checks the length before allocating memory for the payload.
func readFrame(ctx context.Context, reader io.Reader) ([]byte, error) {
	if err := setDeadline(ctx, reader); err != nil {
		return nil, err
	}
	defer clearDeadline(reader)

	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxFrameSize {
		return nil, fmt.Errorf("invalid control frame size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// writeFrame sends the payload length followed by the payload itself. Both
// parent and bridge use this same layout to separate messages in the stream.
func writeFrame(ctx context.Context, writer io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxFrameSize {
		return fmt.Errorf("invalid control frame size %d", len(payload))
	}
	if err := setDeadline(ctx, writer); err != nil {
		return err
	}
	defer clearDeadline(writer)

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(writer, header[:]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

// writeAll keeps writing until the complete frame has been sent. A writer is
// allowed to accept only part of the provided data in a single call.
func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// setDeadline applies the context deadline when this transport supports one.
// It prevents a read or write from waiting forever after a peer stops making progress.
func setDeadline(ctx context.Context, value any) error {
	setter, ok := value.(deadlineSetter)
	if !ok {
		return nil
	}
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		return nil
	}
	return setter.SetDeadline(deadline)
}

// clearDeadline removes the temporary deadline after one operation completes.
func clearDeadline(value any) {
	if setter, ok := value.(deadlineSetter); ok {
		_ = setter.SetDeadline(time.Time{})
	}
}
