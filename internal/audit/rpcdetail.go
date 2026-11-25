package audit

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// extraMutating are state-changing RPCs whose names do not start with one of
// mutatingPrefixes (stock, POS till and sale operations).
var extraMutating = map[string]string{
	"AdjustStock":     "adjust",
	"TransferStock":   "transfer",
	"ArchiveProduct":  "archive",
	"ImportProducts":  "import",
	"OpenShift":       "open",
	"CloseShift":      "close",
	"CashMove":        "cash-move",
	"HoldSale":        "hold",
	"CompleteSale":    "sale",
	"RefundSale":      "refund",
	"AdjustBalance":   "adjust",
	"ArchiveRegister": "archive",
}

// rpcOverrides replace the derived action for prefixed methods.
var rpcOverrides = map[string]string{
	"DeleteHeldSale": "void",
}

// field reads a scalar field from a proto message by name ("" when absent).
func field(m proto.Message, name string) string {
	if m == nil {
		return ""
	}
	r := m.ProtoReflect()
	fd := r.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil || fd.IsList() || fd.IsMap() || fd.Message() != nil {
		return ""
	}
	v := r.Get(fd)
	switch fd.Kind() {
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BoolKind:
		return fmt.Sprint(v.Bool())
	case protoreflect.DoubleKind, protoreflect.FloatKind:
		return fmt.Sprint(v.Float())
	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return fmt.Sprint(v.Int())
	}
	return ""
}

func lineCount(m proto.Message, name string) int {
	if m == nil {
		return 0
	}
	r := m.ProtoReflect()
	fd := r.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil || !fd.IsList() {
		return 0
	}
	return r.Get(fd).List().Len()
}

// describeRPC maps the inventory/POS operations the audit needs to read
// precisely (stock adjust, sale, refund, void, shift close, price change) to a
// resource type, resource id and a short detail. ok is false for methods that
// have no special mapping.
func describeRPC(method string, req, resp any) (resType, resID string, detail map[string]any, ok bool) {
	rq, _ := req.(proto.Message)
	rs, _ := resp.(proto.Message)
	d := map[string]any{}
	set := func(k, v string) {
		if v != "" {
			d[k] = v
		}
	}
	switch method {
	case "AdjustStock":
		set("qty", field(rq, "qty"))
		set("absolute", field(rq, "absolute"))
		set("reason", field(rq, "reason"))
		set("ref", field(rq, "ref"))
		set("location_id", field(rq, "location_id"))
		return "product", field(rq, "id"), d, true
	case "TransferStock":
		set("qty", field(rq, "qty"))
		set("from", field(rq, "from_location_id"))
		set("to", field(rq, "to_location_id"))
		return "product", field(rq, "id"), d, true
	case "CreateProduct", "UpdateProduct":
		set("name", field(rq, "name"))
		set("sku", field(rq, "sku"))
		set("price", field(rq, "price"))
		set("cost", field(rq, "cost"))
		id := field(rq, "id")
		if id == "" {
			id = field(rs, "id")
		}
		return "product", id, d, true
	case "CompleteSale", "HoldSale":
		set("number", field(rs, "number"))
		set("total", field(rs, "total"))
		set("status", field(rs, "status"))
		set("register_id", field(rq, "register_id"))
		id := field(rs, "id")
		if id == "" {
			id = field(rq, "id")
		}
		return "sale", id, d, true
	case "RefundSale":
		set("method", field(rq, "method"))
		set("restock", field(rq, "restock"))
		set("note", field(rq, "note"))
		set("refunded_total", field(rs, "refunded_total"))
		if n := lineCount(rq, "lines"); n > 0 {
			d["lines"] = n
		}
		return "sale", field(rq, "id"), d, true
	case "DeleteHeldSale":
		return "sale", field(rq, "id"), d, true
	case "OpenShift":
		set("register_id", field(rq, "register_id"))
		set("opening_cash", field(rq, "opening_cash"))
		return "shift", field(rs, "id"), d, true
	case "CloseShift":
		set("counted_cash", field(rq, "counted_cash"))
		set("note", field(rq, "note"))
		return "shift", field(rq, "id"), d, true
	case "CashMove":
		set("amount", field(rq, "amount"))
		set("note", field(rq, "note"))
		return "shift", field(rq, "id"), d, true
	case "AdjustBalance":
		set("kind", field(rq, "kind"))
		set("delta", field(rq, "delta"))
		return "balance", field(rq, "contact_id"), d, true
	}
	return "", "", nil, false
}
