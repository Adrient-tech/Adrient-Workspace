package proto_test

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pdf/proto/audit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pdf/proto/documents"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pdf/proto/signing"
)

// The generated descriptors are raw bytes; a wrong length prefix makes the
// package panic at init or report a different go_package. Load each one and
// check the path, package, go_package option and that services resolve.
func TestDescriptorsLoad(t *testing.T) {
	const base = "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/proto/"
	cases := []struct {
		fd      protoreflect.FileDescriptor
		path    string
		goPkg   string
		service string
	}{
		{audit.File_audit_proto, "audit.proto", base + "audit", "AuditService"},
		{documents.File_documents_proto, "documents.proto", base + "documents", "DocumentsService"},
		{signing.File_signing_proto, "signing.proto", base + "signing", "SigningService"},
	}
	for _, c := range cases {
		if got := c.fd.Path(); got != c.path {
			t.Errorf("path = %q, want %q", got, c.path)
		}
		opts := c.fd.Options()
		if opts == nil {
			t.Fatalf("%s: no options", c.path)
		}
		if got := opts.ProtoReflect().Get(opts.ProtoReflect().Descriptor().Fields().ByName("go_package")).String(); got != c.goPkg {
			t.Errorf("%s: go_package = %q, want %q", c.path, got, c.goPkg)
		}
		if c.fd.Services().ByName(protoreflect.Name(c.service)) == nil {
			t.Errorf("%s: service %s missing", c.path, c.service)
		}
		if c.fd.Messages().Len() == 0 {
			t.Errorf("%s: no messages", c.path)
		}
	}
}
