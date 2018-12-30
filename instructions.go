package main

import (
	"fmt"
	"strings"

	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/enum"
	"github.com/llir/llvm/ir/types"
)

// TranslateInstruction translates an LLVM instruction to Go.
func TranslateInstruction(inst ir.Instruction) (string, error) {
	switch inst := inst.(type) {
	case *ir.InstAdd:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s + %s", VariableName(inst), x, y), nil

	case *ir.InstAlloca:
		t, err := TypeSpec(inst.ElemType)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.ElemType, err)
		}
		if inst.NElems == nil {
			return fmt.Sprintf("%s = new(%s)", VariableName(inst), t), nil
		}
		nElems, err := FormatValue(inst.NElems)
		if err != nil {
			return "", fmt.Errorf("error translating NElems (%v): %v", inst.NElems, err)
		}
		return fmt.Sprintf("%s = &make([]%s, %s)[0]", VariableName(inst), t, nElems), nil

	case *ir.InstAnd:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		if intType, ok := inst.Typ.(*types.IntType); ok && intType.BitSize == 1 {
			return fmt.Sprintf("%s = %s && %s", VariableName(inst), x, y), nil
		}
		return fmt.Sprintf("%s = %s & %s", VariableName(inst), x, y), nil

	case *ir.InstBitCast:
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		switch inst.From.(type) {
		case *ir.Global:
			from = "&" + from
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = (%s)(unsafe.Pointer(%s))", VariableName(inst), to, from), nil

	case *ir.InstCall:
		callee, err := FormatValue(inst.Callee)
		if err != nil {
			return "", fmt.Errorf("error translating callee (%v): %v", inst.Callee, err)
		}
		args := make([]string, len(inst.Args))
		for i, a := range inst.Args {
			v, err := FormatValue(a)
			if err != nil {
				return "", fmt.Errorf("error translating argument %d (%v): %v", i, a, err)
			}
			args[i] = v
		}
		switch callee {
		case "calloc":
			callee = "libc.Calloc"
		case "free":
			callee = "libc.Free"
		case "ldexp":
			if len(args) == 2 {
				return fmt.Sprintf("%s = math.Ldexp(%s, int(%s))", VariableName(inst), args[0], args[1]), nil
			}
		case "llvm_fabs_f32":
			if len(args) == 1 {
				return fmt.Sprintf("%s = float32(math.Abs(float64(%s)))", VariableName(inst), args[0]), nil
			}
		case "llvm_fabs_f64", "llvm_fabs_f80", "fabs":
			callee = "math.Abs"
		case "llvm_lifetime_start", "llvm_lifetime_end":
			return ";", nil
		case "llvm_memcpy_p0i8_p0i8_i64":
			return fmt.Sprintf("libc.Memmove(%s, %s, %s)", args[0], args[1], args[2]), nil
		case "llvm_memset_p0i8_i64":
			return fmt.Sprintf("libc.Memset(%s, %s, %s)", args[0], args[1], args[2]), nil
		case "llvm_pow_f64":
			callee = "math.Pow"
		case "malloc":
			callee = "libc.Malloc"
		case "memset_pattern16":
			callee = "libc.MemsetPattern16"
		case "printf":
			callee = "noarch.Printf"
		case "putchar":
			if len(args) == 1 {
				return fmt.Sprintf("if _, err := os.Stdout.Write([]byte{byte(%s)}); err != nil { %s = -1 } else { %s = %s }", args[0], VariableName(inst), VariableName(inst), args[0]), nil
			}
		case "strcmp":
			callee = "noarch.Strcmp"
		}
		if types.Equal(inst.Typ, types.Void) {
			return fmt.Sprintf("%s(%s)", callee, strings.Join(args, ", ")), nil
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), callee, strings.Join(args, ", ")), nil

	case *ir.InstFCmp:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}

		var op string
		switch inst.Pred {
		case enum.FPredOEQ:
			op = "=="
		case enum.FPredOGE:
			op = ">="
		case enum.FPredOGT:
			op = ">"
		case enum.FPredOLE:
			op = "<="
		case enum.FPredOLT:
			op = "<"
		case enum.FPredUNE:
			op = "!="
		case enum.FPredORD:
			return fmt.Sprintf("%s = %s == %s && %s == %s", VariableName(inst), x, x, y, y), nil
		case enum.FPredUNO:
			return fmt.Sprintf("%s = %s != %s || %s != %s", VariableName(inst), x, x, y, y), nil
		case enum.FPredUEQ:
			return fmt.Sprintf("%s = %s != %s || %s != %s || %s == %s", VariableName(inst), x, x, y, y, x, y), nil
		case enum.FPredUGT:
			return fmt.Sprintf("%s = %s != %s || %s != %s || %s > %s", VariableName(inst), x, x, y, y, x, y), nil
		case enum.FPredUGE:
			return fmt.Sprintf("%s = %s != %s || %s != %s || %s >= %s", VariableName(inst), x, x, y, y, x, y), nil
		case enum.FPredULT:
			return fmt.Sprintf("%s = %s != %s || %s != %s || %s < %s", VariableName(inst), x, x, y, y, x, y), nil
		case enum.FPredULE:
			return fmt.Sprintf("%s = %s != %s || %s != %s || %s <= %s", VariableName(inst), x, x, y, y, x, y), nil
		case enum.FPredONE:
			return fmt.Sprintf("%s = %s == %s && %s == %s && %s != %s", VariableName(inst), x, x, y, y, x, y), nil
		default:
			return "", fmt.Errorf("unsupported comparison predicate: %v", inst.Pred)
		}

		return fmt.Sprintf("%s = %s %s %s", VariableName(inst), x, op, y), nil

	case *ir.InstFDiv:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s / %s", VariableName(inst), x, y), nil

	case *ir.InstFMul:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s * %s", VariableName(inst), x, y), nil

	case *ir.InstFPExt:
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstFPToSI:
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		if to == "byte" {
			return fmt.Sprintf("%s = byte(int8(%s))", VariableName(inst), from), nil
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstFPTrunc:
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstFSub:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s - %s", VariableName(inst), x, y), nil

	case *ir.InstGetElementPtr:
		result, err := GetElementPtr(inst.ElemType, inst.Src, inst.Indices)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s = %s", VariableName(inst), result), nil

	case *ir.InstICmp:
		var op string
		format := FormatValue
		switch inst.Pred {
		case enum.IPredEQ:
			op = "=="
		case enum.IPredNE:
			op = "!="
		case enum.IPredSGE:
			op = ">="
			format = FormatSigned
		case enum.IPredSGT:
			op = ">"
			format = FormatSigned
		case enum.IPredSLE:
			op = "<="
			format = FormatSigned
		case enum.IPredSLT:
			op = "<"
			format = FormatSigned
		case enum.IPredUGE:
			op = ">="
			format = FormatUnsigned
		case enum.IPredUGT:
			op = ">"
			format = FormatUnsigned
		case enum.IPredULE:
			op = "<="
			format = FormatUnsigned
		case enum.IPredULT:
			op = "<"
			format = FormatUnsigned
		default:
			return "", fmt.Errorf("unsupported comparison predicate: %v", inst.Pred)
		}

		x, err := format(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := format(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s %s %s", VariableName(inst), x, op, y), nil

	case *ir.InstLoad:
		src, err := FormatValue(inst.Src)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.Src, err)
		}
		if _, ok := inst.Src.(*ir.Global); ok {
			return fmt.Sprintf("%s = %s", VariableName(inst), src), nil
		}
		return fmt.Sprintf("%s = *%s", VariableName(inst), src), nil

	case *ir.InstLShr:
		x, err := FormatUnsigned(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatUnsigned(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		if t, ok := inst.Typ.(*types.IntType); ok && t.BitSize > 8 {
			return fmt.Sprintf("%s = int%d(%s >> %s)", VariableName(inst), t.BitSize, x, y), nil
		}
		return fmt.Sprintf("%s = %s >> %s", VariableName(inst), x, y), nil

	case *ir.InstMul:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s * %s", VariableName(inst), x, y), nil

	case *ir.InstOr:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		if intType, ok := inst.Typ.(*types.IntType); ok && intType.BitSize == 1 {
			return fmt.Sprintf("%s = %s || %s", VariableName(inst), x, y), nil
		}
		return fmt.Sprintf("%s = %s | %s", VariableName(inst), x, y), nil

	case *ir.InstPtrToInt:
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = %s(uintptr(unsafe.Pointer(%s)))", VariableName(inst), to, from), nil

	case *ir.InstSDiv:
		x, err := FormatSigned(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatSigned(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		if intType, ok := inst.Typ.(*types.IntType); ok && intType.BitSize == 8 {
			return fmt.Sprintf("%s = byte(%s / %s)", VariableName(inst), x, y), nil
		}
		return fmt.Sprintf("%s = %s / %s", VariableName(inst), x, y), nil

	case *ir.InstSelect:
		cond, err := FormatValue(inst.Cond)
		if err != nil {
			return "", fmt.Errorf("error translating condition (%v): %v", inst.Cond, err)
		}
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating first operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating second operand (%v): %v", inst.Y, err)
		}
		name := VariableName(inst)
		return fmt.Sprintf("if %s { %s = %s } else { %s = %s }", cond, name, x, name, y), nil

	case *ir.InstSExt:
		toType, ok := inst.To.(*types.IntType)
		if !ok {
			return "", fmt.Errorf("unsupported To type for zext: %T", inst.To)
		}
		from, err := FormatSigned(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		return fmt.Sprintf("%s = int%d(%s)", VariableName(inst), toType.BitSize, from), nil

	case *ir.InstShl:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatUnsigned(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s << %s", VariableName(inst), x, y), nil

	case *ir.InstSIToFP:
		from, err := FormatSigned(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstStore:
		dest, err := FormatValue(inst.Dst)
		if err != nil {
			return "", fmt.Errorf("error translating destination (%v): %v", inst.Dst, err)
		}
		src, err := FormatValue(inst.Src)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.Src, err)
		}
		if _, ok := inst.Dst.(*ir.Global); ok {
			return fmt.Sprintf("%s = %s", dest, src), nil
		}
		return fmt.Sprintf("*%s = %s", dest, src), nil

	case *ir.InstSub:
		x, err := FormatValue(inst.X)
		if err != nil {
			return "", fmt.Errorf("error translating left operand (%v): %v", inst.X, err)
		}
		y, err := FormatValue(inst.Y)
		if err != nil {
			return "", fmt.Errorf("error translating right operand (%v): %v", inst.X, err)
		}
		return fmt.Sprintf("%s = %s - %s", VariableName(inst), x, y), nil

	case *ir.InstTrunc:
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating To type (%v): %v", inst.To, err)
		}
		from, err := FormatValue(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstUIToFP:
		from, err := FormatUnsigned(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		to, err := TypeSpec(inst.To)
		if err != nil {
			return "", fmt.Errorf("error translating type (%v): %v", inst.To, err)
		}
		return fmt.Sprintf("%s = %s(%s)", VariableName(inst), to, from), nil

	case *ir.InstZExt:
		toType, ok := inst.To.(*types.IntType)
		if !ok {
			return "", fmt.Errorf("unsupported To type for zext: %T", inst.To)
		}
		from, err := FormatUnsigned(inst.From)
		if err != nil {
			return "", fmt.Errorf("error translating source (%v): %v", inst.From, err)
		}
		if fromType, ok := inst.From.Type().(*types.IntType); ok && fromType.BitSize == 1 {
			return fmt.Sprintf("if %s { %s = 1 } else { %s = 0 }", from, VariableName(inst), VariableName(inst)), nil
		}
		return fmt.Sprintf("%s = int%d(uint%d(%s))", VariableName(inst), toType.BitSize, toType.BitSize, from), nil

	default:
		return "", fmt.Errorf("unsupported instruction type: %T", inst)
	}
}
