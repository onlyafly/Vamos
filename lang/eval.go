package lang

import ()

////////// Trampoline Support

type packet struct {
	Thunk thunk
	Node  Node
}

func bounce(t thunk) packet {
	return packet{Thunk: t}
}

func respond(n Node) packet {
	return packet{Node: n}
}

type thunk func() packet

func trampoline(currentThunk thunk) Node {
	for currentThunk != nil {
		next := currentThunk()

		if next.Thunk != nil {
			currentThunk = next.Thunk
		} else {
			return next.Node
		}
	}

	return nil
}

////////// Evaluation

func Eval(e Env, n Node) (result Node, err error) {
	defer func() {
		if e := recover(); e != nil {
			result = nil
			switch errorValue := e.(type) {
			case EvalError:
				err = errorValue
				return
			default:
				panic(errorValue)
			}
		}
	}()

	startThunk := func() packet {
		return evalNode(e, n)
	}

	return trampoline(startThunk), nil
}

func evalEachNode(e Env, ns []Node) []Node {
	result := make([]Node, len(ns))
	for i, n := range ns {
		evalNodeThunk := func() packet {
			return evalNode(e, n)
		}
		result[i] = trampoline(evalNodeThunk)
	}
	return result
}

func evalNode(e Env, n Node) packet {

	switch value := n.(type) {
	case *Number:
		return respond(value)
	case *Symbol:
		return respond(e.Get(value.Name))
	case *List:
		return bounce(func() packet { return evalList(e, value) })
	default:
		panicEvalError("Unknown form to evaluate: " + value.String())
	}

	return respond(&Symbol{Name: "nil"})
}

func evalList(e Env, l *List) packet {
	elements := l.Nodes

	if len(elements) == 0 {
		panicEvalError("Empty list cannot be evaluated: " + l.String())
		return respond(nil)
	}

	head := elements[0]
	args := elements[1:]

	switch value := head.(type) {
	case *Symbol:
		switch value.Name {
		case "def":
			name := toSymbolName(args[0])
			e.Set(name, trampoline(func() packet {
				return evalNode(e, args[1])
			}))
			return respond(&Symbol{Name: "nil"})
		case "if":
			predicate := toBooleanValue(trampoline(func() packet {
				return evalNode(e, args[0])
			}))
			if predicate {
				return bounce(func() packet {
					return evalNode(e, args[1])
				})
			} else {
				return bounce(func() packet {
					return evalNode(e, args[2])
				})
			}
		case "cond":
			for i := 0; i < len(args); i += 2 {
				predicate := toBooleanValue(trampoline(func() packet {
					return evalNode(e, args[i])
				}))

				if predicate {
					return bounce(func() packet {
						return evalNode(e, args[i+1])
					})
				}
			}
			panicEvalError("No matching cond clause: " + l.String())
		case "fn":
			return bounce(func() packet {
				return evalFunctionDefinition(e, args)
			})
		case "quote":
			return respond(args[0])
		case "let":
			return bounce(func() packet {
				return evalLet(e, args)
			})
		}
	}

	var headNode Node = trampoline(func() packet {
		return evalNode(e, head)
	})

	switch value := headNode.(type) {
	case *Primitive:
		f := value.Value
		return respond(f(evalEachNode(e, args)))
	case *Function:
		arguments := evalEachNode(e, args)

		return bounce(func() packet {
			return evalFunctionApplication(value, arguments)
		})
	default:
		panicEvalError("First item in list not a function: " + value.String())
	}

	return respond(&Symbol{Name: "nil"})
}

func evalFunctionApplication(f *Function, args []Node) packet {

	var e Env = NewMapEnv(f.Name, f.ParentEnv)

	// TODO
	/*
		print(
			"evalFunctionApplication:\n   name=",
			e.String(), "\n   body=",
			f.Body.String(), "\n   parent=",
			f.ParentEnv.String(), "\n   args=",
			fmt.Sprintf("%v", args), "\n   isTail=",
			fmt.Sprintf("%v", isTail), "\n")
	*/

	// Save arguments into parameters
	for i, arg := range args {
		paramName := toSymbolName(f.Parameters[i])
		e.Set(paramName, arg)
	}

	return bounce(func() packet {
		return evalNode(e, f.Body)
	})
}

func evalFunctionDefinition(e Env, args []Node) packet {
	parameterList := args[0]
	parameterNodes := parameterList.Children()

	return respond(&Function{
		Name:       "anonymous",
		Parameters: parameterNodes,
		Body:       args[1],
		ParentEnv:  e,
	})
}

func evalLet(parentEnv Env, args []Node) packet {
	variableList := args[0]
	body := args[1]
	variableNodes := variableList.Children()

	var e Env = NewMapEnv("let", parentEnv)

	// Evaluate variable assignments
	for i := 0; i < len(variableNodes); i += 2 {
		variable := variableNodes[i]
		expression := variableNodes[i+1]
		variableName := toSymbolName(variable)

		e.Set(variableName, trampoline(func() packet {
			return evalNode(e, expression)
		}))
	}

	// Evaluate body
	return bounce(func() packet {
		return evalNode(e, body)
	})
}

func panicEvalError(s string) {
	panic(EvalError(s))
}

func toSymbolName(n Node) string {
	switch value := n.(type) {
	case *Symbol:
		return value.Name
	}

	panic("Not a symbol: " + n.String())
}