package lintsensitivity

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// MaxESLintReceiptBytes bounds the whole internal frame, including its header.
// It is not a second public evidence format or an admitted launcher protocol.
const MaxESLintReceiptBytes = 64 << 10

const receiptMagic = "CI-TRUST-ESLINT-RECEIPT/1"

var invalidReceipt = errors.New("invalid owned ESLint receipt")

// EncodeESLintReceipt frames supplied tool output for the owned launcher candidate.
// Completed is a caller assertion, not verified by encoding. The expected binding
// must eventually cover the exact invocation/role/runtime; syntax is not authority.
// No process, file, approval or runtime admission operation occurs here.
func EncodeESLintReceipt(role Role, binding string, tool ESLintCapture) ([]byte, error) {
	if _, ok := FixtureFor(role); !ok || !sha.MatchString(binding) || tool.Status != Completed ||
		(tool.ExitCode != 0 && tool.ExitCode != 1) || len(tool.Stdout) > MaxESLintReceiptBytes ||
		len(tool.Stderr) > MaxESLintReceiptBytes-len(tool.Stdout) {
		return nil, invalidReceipt
	}
	header := fmt.Sprintf("%s\n%s\n%s\n%d\n%d\n%d\n", receiptMagic, binding, role, tool.ExitCode, len(tool.Stdout), len(tool.Stderr))
	if len(header) > MaxESLintReceiptBytes-len(tool.Stdout)-len(tool.Stderr) {
		return nil, invalidReceipt
	}
	out := make([]byte, 0, len(header)+len(tool.Stdout)+len(tool.Stderr))
	out = append(out, header...)
	out = append(out, tool.Stdout...)
	return append(out, tool.Stderr...), nil
}

// DecodeESLintReceipt consumes an outer launcher/transport capture, not a direct
// ESLint capture. Completed must independently assert trusted execution, verified
// cleanup and source preservation. The outer exit must be zero, stderr empty.
// A valid frame cannot override any failed outer status or authenticate its own
// origin. After framing, the existing strict tool parser still decides validity.
func DecodeESLintReceipt(role Role, id Identity, binding string, outer ESLintCapture) Observation {
	if outer.Status != Completed {
		return DecodeESLint(role, id, outer)
	}
	invalid := func() Observation {
		return DecodeESLint(role, id, ESLintCapture{Status: InvalidOutput, ExitCode: outer.ExitCode})
	}
	if _, ok := FixtureFor(role); !ok || !sha.MatchString(binding) || outer.ExitCode != 0 ||
		len(outer.Stderr) != 0 || len(outer.Stdout) > MaxESLintReceiptBytes {
		return invalid()
	}
	parts := bytes.SplitN(outer.Stdout, []byte{'\n'}, 7)
	if len(parts) != 7 || string(parts[0]) != receiptMagic || string(parts[1]) != binding || string(parts[2]) != string(role) ||
		(len(parts[3]) != 1 || (parts[3][0] != '0' && parts[3][0] != '1')) {
		return invalid()
	}
	stdout, ok := receiptLength(parts[4])
	if !ok {
		return invalid()
	}
	stderr, ok := receiptLength(parts[5])
	if !ok || stderr > MaxESLintReceiptBytes-stdout || len(parts[6]) != stdout+stderr {
		return invalid()
	}
	return DecodeESLint(role, id, ESLintCapture{
		Status: Completed, ExitCode: int(parts[3][0] - '0'),
		Stdout: parts[6][:stdout], Stderr: parts[6][stdout:],
	})
}

func receiptLength(raw []byte) (int, bool) {
	if len(raw) < 1 || len(raw) > 5 {
		return 0, false
	}
	n, err := strconv.Atoi(string(raw))
	return n, err == nil && n >= 0 && n <= MaxESLintReceiptBytes && strconv.Itoa(n) == string(raw)
}
