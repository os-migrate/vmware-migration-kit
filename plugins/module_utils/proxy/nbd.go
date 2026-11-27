/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2026 Red Hat, Inc.
 *
 */

package proxy

// Minimal NBD newstyle client: just enough of the handshake to ask a server
// about one export (NBD_OPT_INFO) and read back its size and flags.
//
// Implemented here rather than with libnbd on purpose: this package is linked
// into the module_dispatcher binary, which must stay free of cgo.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const (
	nbdOptionMagic = uint64(0x49484156454F5054) // "IHAVEOPT"
	nbdReplyMagic  = uint64(0x3e889045565a9)

	nbdOptInfo  = uint32(6)
	nbdOptAbort = uint32(2)

	nbdRepAck  = uint32(1)
	nbdRepInfo = uint32(3)
	// Reply types with the high bit set are errors.
	nbdRepError        = uint32(1) << 31
	nbdRepErrUnknown   = nbdRepError + 6 // export not available
	nbdInfoExport      = uint16(0)
	nbdFlagFixedNewstl = uint32(1)
	nbdFlagReadOnly    = uint16(1) << 1

	// Replies are small; refuse anything absurd rather than allocating it.
	nbdMaxReplyLen = uint32(4096)
)

// ErrExportNotFound is returned when the server does not serve that export.
// With the proxy transport it means the hot-added disk has not shown up in
// the proxy yet (SCSI rescan and udev are asynchronous).
var ErrExportNotFound = errors.New("NBD export not found")

// ExportInfo is what a server reports about an export.
type ExportInfo struct {
	Size     uint64
	ReadOnly bool
}

// QueryExport asks the NBD server at address:port about the named export.
// It only runs the handshake (NBD_OPT_INFO then NBD_OPT_ABORT): no data is
// transferred and the export is not opened.
func QueryExport(address string, port int, name string, timeout time.Duration) (ExportInfo, error) {
	var info ExportInfo
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(address, strconv.Itoa(port)), timeout)
	if err != nil {
		return info, err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return info, err
	}

	// Server greeting: "NBDMAGIC" "IHAVEOPT" <handshake flags>
	greeting := make([]byte, len(nbdMagic)+2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return info, fmt.Errorf("no NBD greeting: %w", err)
	}
	if string(greeting[:len(nbdMagic)]) != string(nbdMagic) {
		return info, fmt.Errorf("unexpected NBD greeting %q", greeting[:len(nbdMagic)])
	}

	// Client flags, then NBD_OPT_INFO <name> with no info requests.
	if err := binary.Write(conn, binary.BigEndian, nbdFlagFixedNewstl); err != nil {
		return info, err
	}
	payload := make([]byte, 0, 4+len(name)+2)
	payload = binary.BigEndian.AppendUint32(payload, uint32(len(name)))
	payload = append(payload, name...)
	payload = binary.BigEndian.AppendUint16(payload, 0)
	if err := writeOption(conn, nbdOptInfo, payload); err != nil {
		return info, err
	}

	// Replies until NBD_REP_ACK (success) or an error reply.
	found := false
	for {
		repType, data, err := readReply(conn)
		if err != nil {
			return info, err
		}
		switch {
		case repType == nbdRepAck:
			_ = writeOption(conn, nbdOptAbort, nil)
			if !found {
				return info, fmt.Errorf("server acknowledged export %q without describing it", name)
			}
			return info, nil
		case repType == nbdRepInfo:
			if len(data) < 2 || binary.BigEndian.Uint16(data) != nbdInfoExport {
				continue // another info type, not the export description
			}
			if len(data) < 12 {
				return info, fmt.Errorf("truncated NBD_INFO_EXPORT reply (%d bytes)", len(data))
			}
			info.Size = binary.BigEndian.Uint64(data[2:10])
			info.ReadOnly = binary.BigEndian.Uint16(data[10:12])&nbdFlagReadOnly != 0
			found = true
		case repType == nbdRepErrUnknown:
			_ = writeOption(conn, nbdOptAbort, nil)
			return info, fmt.Errorf("%w: %q", ErrExportNotFound, name)
		case repType&nbdRepError != 0:
			_ = writeOption(conn, nbdOptAbort, nil)
			return info, fmt.Errorf("NBD server refused export %q: error 0x%x %s", name, repType, data)
		default:
			return info, fmt.Errorf("unexpected NBD reply type 0x%x", repType)
		}
	}
}

func writeOption(w io.Writer, option uint32, payload []byte) error {
	buf := make([]byte, 0, 16+len(payload))
	buf = binary.BigEndian.AppendUint64(buf, nbdOptionMagic)
	buf = binary.BigEndian.AppendUint32(buf, option)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(payload)))
	buf = append(buf, payload...)
	_, err := w.Write(buf)
	return err
}

// readReply reads one option reply: magic, option, type, length, payload.
func readReply(r io.Reader) (uint32, []byte, error) {
	header := make([]byte, 20)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, fmt.Errorf("truncated NBD reply: %w", err)
	}
	if binary.BigEndian.Uint64(header[:8]) != nbdReplyMagic {
		return 0, nil, fmt.Errorf("bad NBD reply magic 0x%x", binary.BigEndian.Uint64(header[:8]))
	}
	repType := binary.BigEndian.Uint32(header[12:16])
	length := binary.BigEndian.Uint32(header[16:20])
	if length > nbdMaxReplyLen {
		return 0, nil, fmt.Errorf("NBD reply too long (%d bytes)", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, fmt.Errorf("truncated NBD reply payload: %w", err)
	}
	return repType, data, nil
}

// WaitExport waits for the named export to be served, then checks it is
// read-only and has the expected size. A hot-added disk takes a moment to
// appear in the proxy, so a missing export is retried until the timeout.
func WaitExport(ctx context.Context, address string, port int, name string, capacity int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		info, err := QueryExport(address, port, name, 10*time.Second)
		switch {
		case err == nil:
			if info.Size != uint64(capacity) {
				return fmt.Errorf("NBD export %s is %d bytes, expected %d: wrong or stale device in the proxy",
					name, info.Size, capacity)
			}
			if !info.ReadOnly {
				return fmt.Errorf("NBD export %s is writable, it must be read-only "+
					"(the nbdkit service of the proxy must run with --readonly)", name)
			}
			return nil
		case !errors.Is(err, ErrExportNotFound) && time.Now().After(deadline):
			return fmt.Errorf("cannot query NBD export %s on %s:%d "+
				"(is the nbdkit service running on the proxy and the port open for this host?): %w",
				name, address, port, err)
		case time.Now().After(deadline):
			return fmt.Errorf("NBD export %s did not appear on %s:%d within %s: "+
				"the disk was attached to the proxy but did not show up there "+
				"(check disk.EnableUUID=TRUE and that nbdkit serves dir=/dev/disk/by-id)",
				name, address, port, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
