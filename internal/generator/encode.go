package generator

import (
	"fmt"
	"strings"

	gleamfmt "github.com/qwexvf/protoc-gen-gleam/internal/gleam"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func generateEncoder(ctx *genContext, msg *protogen.Message, typeName string, oneofs []oneofInfo) {
	fnName := "encode_" + gleamfmt.ToSnakeCase(typeName)
	regular := regularFields(msg)

	if len(regular) == 0 && len(oneofs) == 0 {
		ctx.w.P("pub fn %s(_msg: %s) -> BitArray {", fnName, typeName)
		ctx.w.P("  <<>>")
		ctx.w.P("}")
		ctx.w.P("")
		return
	}

	ctx.w.P("pub fn %s(msg: %s) -> BitArray {", fnName, typeName)

	// Destructure.
	var fieldNames []string
	for _, f := range regular {
		fieldNames = append(fieldNames, gleamfmt.FieldName(string(f.Desc.Name())))
	}
	for _, oo := range oneofs {
		fieldNames = append(fieldNames, gleamfmt.FieldName(string(oo.Desc.Name())))
	}
	ctx.w.P("  let %s(%s) = msg", typeName, joinComma(fieldNames))

	ctx.w.P("  <<")
	for _, f := range regular {
		fieldName := gleamfmt.FieldName(string(f.Desc.Name()))
		constName := "field_" + gleamfmt.ToSnakeCase(typeName) + "_" + string(f.Desc.Name())
		ctx.w.P("    %s,", encodeFieldExpr(ctx, f, fieldName, constName))
	}
	for _, oo := range oneofs {
		ooFieldName := gleamfmt.FieldName(string(oo.Desc.Name()))
		encoderName := oneofEncoderName(typeName, oo)
		ctx.w.P("    %s(%s):bits,", encoderName, ooFieldName)
	}
	ctx.w.P("  >>")
	ctx.w.P("}")
	ctx.w.P("")

	for _, oo := range oneofs {
		generateOneofEncoder(ctx, oo, typeName)
	}
}

func generateOneofEncoder(ctx *genContext, oo oneofInfo, parentTypeName string) {
	ooTypeName := oneofTypeName(parentTypeName, oo)
	fnName := oneofEncoderName(parentTypeName, oo)

	ctx.w.P("fn %s(p: %s) -> BitArray {", fnName, ooTypeName)
	ctx.w.P("  case p {")
	for _, f := range oo.Fields {
		variantName := oneofVariantName(parentTypeName, oo, f)
		constName := oneofConstName(parentTypeName, f)
		// A set oneof member has presence, so zero values must be written.
		ctx.w.P("    %s(m) -> %s", variantName, encodeScalarCall(ctx, f, "m", constName, true))
	}
	ctx.w.P("  }")
	ctx.w.P("}")
	ctx.w.P("")
}

func encodeFieldExpr(ctx *genContext, f *protogen.Field, fieldName, constName string) string {
	// Map fields.
	if isMap(f) {
		return encodeMapExpr(ctx, f, fieldName, constName)
	}

	// Optional fields have presence: write the value when set, even if zero.
	if isOptional(f) {
		return fmt.Sprintf("wire.encode_optional(%s, fn(v) { %s }):bits", fieldName, encodeScalarCall(ctx, f, "v", constName, true))
	}

	// Repeated fields.
	if f.Desc.IsList() {
		return encodeRepeatedExpr(ctx, f, fieldName, constName)
	}

	// Scalar fields.
	return encodeScalarCall(ctx, f, fieldName, constName, false) + ":bits"
}

// encodeScalarCall returns a call that encodes one singular value. With
// always set it writes zero values too, for fields with explicit presence;
// otherwise zero values are omitted (proto3 default-value rule).
func encodeScalarCall(ctx *genContext, f *protogen.Field, fieldName, constName string, always bool) string {
	suffix := ""
	if always {
		suffix = "_always"
	}
	var fn, value string
	switch f.Desc.Kind() {
	case protoreflect.StringKind:
		fn, value = "string", fieldName
	case protoreflect.BoolKind:
		fn, value = "bool", fieldName
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		fn, value = "int", fieldName
	case protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		fn, value = "sint", fieldName
	case protoreflect.Fixed32Kind:
		fn, value = "fixed32", fieldName
	case protoreflect.Fixed64Kind:
		fn, value = "fixed64", fieldName
	case protoreflect.Sfixed32Kind:
		fn, value = "sfixed32", fieldName
	case protoreflect.Sfixed64Kind:
		fn, value = "sfixed64", fieldName
	case protoreflect.FloatKind:
		fn, value = "float", fieldName
	case protoreflect.DoubleKind:
		fn, value = "double", fieldName
	case protoreflect.BytesKind:
		fn, value = "bytes", fieldName
	case protoreflect.EnumKind:
		fn, value = "int", fmt.Sprintf("%s_to_int(%s)", enumFnPrefix(ctx, f), fieldName)
	case protoreflect.MessageKind:
		fn, value = "message", fmt.Sprintf("%s(%s)", encoderFnName(ctx, f.Message), fieldName)
	default:
		// Only proto2 groups reach here, and proto2 is unsupported.
		panic(fmt.Sprintf("unsupported field kind %s for %s", f.Desc.Kind(), f.Desc.FullName()))
	}
	return fmt.Sprintf("wire.encode_%s_field%s(%s, %s)", fn, suffix, constName, value)
}

func encodeRepeatedExpr(ctx *genContext, f *protogen.Field, fieldName, constName string) string {
	switch f.Desc.Kind() {
	case protoreflect.StringKind:
		return fmt.Sprintf("wire.encode_repeated_strings(%s, %s):bits", constName, fieldName)
	case protoreflect.MessageKind:
		enc := encoderFnName(ctx, f.Message)
		return fmt.Sprintf("wire.encode_repeated_messages(%s, %s, %s):bits", constName, fieldName, enc)
	case protoreflect.BytesKind:
		return fmt.Sprintf("wire.encode_repeated_messages(%s, %s, fn(b) { b }):bits", constName, fieldName)
	case protoreflect.EnumKind:
		prefix := enumFnPrefix(ctx, f)
		return fmt.Sprintf("wire.encode_repeated_ints(%s, list.map(%s, %s_to_int)):bits", constName, fieldName, prefix)
	case protoreflect.BoolKind:
		return fmt.Sprintf("wire.encode_repeated_ints(%s, list.map(%s, fn(b) { case b { True -> 1 False -> 0 } })):bits", constName, fieldName)
	default:
		// int32, int64, uint32, uint64, sint32, sint64, etc.
		return fmt.Sprintf("wire.encode_repeated_ints(%s, %s):bits", constName, fieldName)
	}
}

func encodeMapExpr(ctx *genContext, f *protogen.Field, fieldName, constName string) string {
	mapEntry := f.Message
	keyField := mapEntry.Fields[0]
	valField := mapEntry.Fields[1]

	keyEnc := mapKeyEncoder(ctx, keyField)
	valEnc := mapValueEncoder(ctx, valField)

	return fmt.Sprintf("wire.encode_map_entries(%s, dict.to_list(%s), %s, %s):bits",
		constName, fieldName, keyEnc, valEnc)
}

func mapKeyEncoder(_ *genContext, f *protogen.Field) string {
	switch f.Desc.Kind() {
	case protoreflect.StringKind:
		return "wire.encode_string_field"
	case protoreflect.BoolKind:
		return "wire.encode_bool_field"
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return "wire.encode_int_field"
	case protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return "wire.encode_sint_field"
	case protoreflect.Fixed32Kind:
		return "wire.encode_fixed32_field"
	case protoreflect.Fixed64Kind:
		return "wire.encode_fixed64_field"
	case protoreflect.Sfixed32Kind:
		return "wire.encode_sfixed32_field"
	case protoreflect.Sfixed64Kind:
		return "wire.encode_sfixed64_field"
	default:
		return "wire.encode_string_field // unsupported map key type"
	}
}

func mapValueEncoder(ctx *genContext, f *protogen.Field) string {
	switch f.Desc.Kind() {
	case protoreflect.StringKind:
		return "wire.encode_string_field"
	case protoreflect.BoolKind:
		return "wire.encode_bool_field"
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return "wire.encode_int_field"
	case protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return "wire.encode_sint_field"
	case protoreflect.FloatKind:
		return "wire.encode_float_field"
	case protoreflect.DoubleKind:
		return "wire.encode_double_field"
	case protoreflect.BytesKind:
		return "wire.encode_bytes_field"
	case protoreflect.Fixed32Kind:
		return "wire.encode_fixed32_field"
	case protoreflect.Fixed64Kind:
		return "wire.encode_fixed64_field"
	case protoreflect.Sfixed32Kind:
		return "wire.encode_sfixed32_field"
	case protoreflect.Sfixed64Kind:
		return "wire.encode_sfixed64_field"
	case protoreflect.MessageKind:
		enc := encoderFnName(ctx, f.Message)
		return fmt.Sprintf("fn(fn_num, v) { wire.encode_message_field(fn_num, %s(v)) }", enc)
	case protoreflect.EnumKind:
		prefix := enumFnPrefix(ctx, f)
		return fmt.Sprintf("fn(fn_num, v) { wire.encode_int_field(fn_num, %s_to_int(v)) }", prefix)
	default:
		return "wire.encode_string_field // unsupported map value type"
	}
}

func joinComma(ss []string) string {
	return strings.Join(ss, ", ")
}
