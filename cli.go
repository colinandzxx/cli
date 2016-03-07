package cli

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/labstack/gommon/color"
)

// Run runs a single command app
func Run(argv interface{}, fn CommandFunc) {
	err := (&Command{
		Name: os.Args[0],
		Argv: func() interface{} { return argv },
		Fn:   fn,
	}).Run(os.Args[1:])
	if err != nil {
		fmt.Println(err)
	}
}

// Root registers forest for root and return root
func Root(root *Command, forest ...*CommandTree) *Command {
	root.RegisterTree(forest...)
	return root
}

// Tree creates a CommandTree
func Tree(cmd *Command, forest ...*CommandTree) *CommandTree {
	return &CommandTree{
		command: cmd,
		forest:  forest,
	}
}

//-----------------------------
// Implements parse and others
//-----------------------------

func parseArgv(args []string, argv interface{}, clr color.Color) *flagSet {
	var (
		typ     = reflect.TypeOf(argv)
		val     = reflect.ValueOf(argv)
		flagSet = newFlagSet()
	)
	switch typ.Kind() {
	case reflect.Ptr:
		if reflect.Indirect(val).Type().Kind() != reflect.Struct {
			flagSet.err = errNotPointToStruct
			return flagSet
		}
		parse(args, typ, val, flagSet, clr)
		return flagSet
	default:
		flagSet.err = errNotAPointer
		return flagSet
	}
}

func usage(v interface{}, clr color.Color) string {
	var (
		typ     = reflect.TypeOf(v)
		val     = reflect.ValueOf(v)
		flagSet = newFlagSet()
	)
	if typ.Kind() == reflect.Ptr {
		if reflect.Indirect(val).Type().Kind() == reflect.Struct {
			initFlagSet(typ, val, flagSet, clr)
			if flagSet.err != nil {
				return ""
			}
			return flagSlice(flagSet.flags).String(clr)
		}
	}
	return ""
}

func initFlagSet(typ reflect.Type, val reflect.Value, flagSet *flagSet, clr color.Color) {
	var (
		tm       = typ.Elem()
		vm       = val.Elem()
		fieldNum = vm.NumField()
	)
	for i := 0; i < fieldNum; i++ {
		tfield := tm.Field(i)
		vfield := vm.Field(i)
		tag, isEmpty := parseTag(tfield.Name, tfield.Tag)
		if tag == nil {
			continue
		}
		// if `cli` tag is empty and the field is a struct
		if isEmpty && vfield.Kind() == reflect.Struct {
			subObj := vfield.Addr().Interface()
			initFlagSet(reflect.TypeOf(subObj), reflect.ValueOf(subObj), flagSet, clr)
			if flagSet.err != nil {
				return
			}
			continue
		}
		fl, err := newFlag(tfield, vfield, tag, clr)
		if flagSet.err = err; err != nil {
			return
		}
		// Ignored flag
		if fl == nil {
			continue
		}
		flagSet.flags = append(flagSet.flags, fl)
		value := ""
		if fl.assigned {
			value = fmt.Sprintf("%v", vfield.Interface())
		}

		names := append(fl.tag.shortNames, fl.tag.longNames...)
		for i, name := range names {
			if _, ok := flagSet.flagMap[name]; ok {
				flagSet.err = fmt.Errorf("flag %s repeat", clr.Bold(name))
				return
			}
			flagSet.flagMap[name] = fl
			if fl.assigned && i == 0 {
				flagSet.values[name] = []string{value}
			}
		}
	}
}

func parse(args []string, typ reflect.Type, val reflect.Value, flagSet *flagSet, clr color.Color) {
	initFlagSet(typ, val, flagSet, clr)
	if flagSet.err != nil {
		return
	}

	size := len(args)
	for i := 0; i < size; i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, dashOne) {
			continue
		}
		values := []string{}
		for j := i + 1; j < size; j++ {
			if strings.HasPrefix(args[j], dashOne) {
				break
			}
			values = append(values, args[j])
		}
		i += len(values)

		strs := strings.Split(arg, "=")
		if strs == nil || len(strs) == 0 {
			continue
		}

		arg = strs[0]
		fl, ok := flagSet.flagMap[arg]
		if !ok {
			// If has prefix `--`
			if strings.HasPrefix(arg, dashTwo) {
				flagSet.err = fmt.Errorf("undefined flag %s", clr.Bold(arg))
				return
			}
			// Else find arg char by char
			chars := []byte(strings.TrimPrefix(arg, dashOne))
			for _, c := range chars {
				tmp := dashOne + string([]byte{c})
				fl, ok := flagSet.flagMap[tmp]
				if !ok {
					flagSet.err = fmt.Errorf("undefined flag %s", clr.Bold(tmp))
					return
				}

				if flagSet.err = fl.set(tmp, "", clr); flagSet.err != nil {
					return
				}
				if fl.err == nil {
					flagSet.values[tmp] = []string{fmt.Sprintf("%v", fl.v.Interface())}
				}

			}
			continue
		}

		values = append(strs[1:], values...)
		if len(values) == 0 {
			flagSet.err = fl.set(arg, "", clr)
		} else if len(values) == 1 {
			flagSet.err = fl.set(arg, values[0], clr)
		} else {
			flagSet.err = fmt.Errorf("too many(%d) value for flag %s", len(values), clr.Bold(arg))
		}
		if flagSet.err != nil {
			return
		}
		if fl.err == nil {
			flagSet.values[arg] = []string{fmt.Sprintf("%v", fl.v.Interface())}
		} else if fl.assigned {
			flagSet.err = fmt.Errorf("assigned argument %s invalid: %v", clr.Bold(fl.name()), fl.err)
			return
		}
	}

	buff := bytes.NewBufferString("")
	for _, fl := range flagSet.flags {
		if !fl.assigned && fl.tag.required {
			if buff.Len() > 0 {
				buff.WriteByte('\n')
			}
			fmt.Fprintf(buff, "required argument %s missing", clr.Bold(fl.name()))
		}
	}
	for _, fl := range flagSet.flags {
		if fl.tag.isHelp && fl.getBool() {
			flagSet.dontValidate = true
			break
		}
	}
	if buff.Len() > 0 && !flagSet.dontValidate {
		flagSet.err = fmt.Errorf(buff.String())
	}
}
