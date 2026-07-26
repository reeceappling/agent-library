package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tmc/langchaingo/llms"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (agent *LLMAgent) toolOptions() llms.CallOption {
	return func(co *llms.CallOptions) {
		co.Tools = agent.toolkit.l
	}
}
func (agent *LLMAgent) promptRecursive(ctx context.Context, shortAndLongTermMessages []llms.MessageContent) (string, error) {
	println("RECEIVED RECURSIVE PROMPT")
	// TODO: what to do with old (agent-internal) messages?
	resp, err := agent.model.GenerateContent(ctx, shortAndLongTermMessages, agent.toolOptions()) // TODO: OPTS???
	if err != nil {
		errMsg := "failed to prompt agent"
		shortAndLongTermMessages = append(shortAndLongTermMessages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI, // TODO: ok?? or system?
			Parts: []llms.ContentPart{llms.TextPart(errMsg)},
		})
		return errMsg, err
	}
	if len(resp.Choices) != 1 {
		errMsg := "handling multiple choices is not implemented yet"
		shortAndLongTermMessages = append(shortAndLongTermMessages, llms.MessageContent{ // TODO: do we really want to append on to the agent's current messages here?
			Role:  llms.ChatMessageTypeSystem, // TODO: llms.ChatMessageTypeFunction or llms.ChatMessageTypeTool?
			Parts: []llms.ContentPart{llms.TextPart(errMsg)},
		})
		return errMsg, errors.New(errMsg)
	}
	// HANDLE TOOL RESPONSES
	choice := resp.Choices[0]
	if len(choice.ToolCalls) == 0 {
		return strings.TrimRight(choice.Content, "\r\n"), nil // TODO: ensure ok!
	}
	toolCallResults := make([]llms.MessageContent, 0, len(choice.ToolCalls))
	for _, tc := range choice.ToolCalls {
		handler, exists := agent.toolkit.HandlerForTool(tc.FunctionCall.Name)
		if !exists {
			toolCallResults = append(toolCallResults, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    "Error: tool does not exist in toolkit. All calls to this tool will fail",
					},
				},
			})
			return agent.promptRecursive(ctx, shortAndLongTermMessages)
		}

		toolHandlerResponse, err := handler(ctx, tc.FunctionCall.Arguments)
		if err != nil {
			toolHandlerResponse = "Tool handler error: " + err.Error() // TODO: don't overwrite???
		}
		toolCallResults = append(toolCallResults, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: tc.ID,
					Name:       tc.FunctionCall.Name,
					Content:    toolHandlerResponse,
				},
			},
		})
	}
	// TODO: handle  resp.Choices[0].Content in the new query as well?
	shortAndLongTermMessages = append(shortAndLongTermMessages, llms.MessageContent{ // TODO: do we really want to append on to the agent's current messages here?
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{llms.TextPart(resp.Choices[0].Content)},
	})
	shortAndLongTermMessages = append(shortAndLongTermMessages, toolCallResults...) // TODO: do we really want to append on to the agent's current messages here?
	// TODO: add anything else on to the response that is necessary
	return agent.promptRecursive(ctx, shortAndLongTermMessages)
}

func (agent *LLMAgent) Prompt(ctx context.Context, thisPromptMessages []llms.MessageContent) (string, error) {
	println("RECEIVED TOP LEVEL AGENT PROMPT")
	// TODO: ENSURE TO REMOVE ANY NEWLINES AT THE END OF RESPONSES (IF NEEDED)!!!!
	if agent.messages == nil || len(agent.messages) == 0 { // TODO: ok?
		agent.messages = []llms.MessageContent{}
	}
	longTermMessages := agent.messages
	if len(longTermMessages) == 0 {
		// TODO: ?????
	}
	shortTermMessages := thisPromptMessages
	messages := make([]llms.MessageContent, 0, len(longTermMessages)+len(shortTermMessages))
	messages = append(messages, longTermMessages...)
	messages = append(messages, shortTermMessages...)
	// TODO: what to do with old (agent-internal) messages?
	resp, err := agent.model.GenerateContent(ctx, thisPromptMessages, agent.toolOptions()) // TODO: OPTS???
	if err != nil {
		errMsg := "failed to GenerateContent"
		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextPart(errMsg)},
		})
		return errMsg, errors.Join(errors.New(errMsg), err)
	}
	if len(resp.Choices) != 1 {
		errMsg := "handling multiple choices is not implemented yet"
		messages = append(messages, llms.MessageContent{ // TODO: do we really want to append on to the agent's current messages here?
			Role:  llms.ChatMessageTypeSystem, // TODO: llms.ChatMessageTypeFunction or llms.ChatMessageTypeTool?
			Parts: []llms.ContentPart{llms.TextPart(errMsg)},
		})
		return errMsg, errors.New(errMsg)
	}
	// HANDLE TOOL RESPONSES
	choice := resp.Choices[0]
	if len(choice.ToolCalls) == 0 {
		return strings.TrimRight(choice.Content, "\r\n"), nil
	}
	toolCallResults := make([]llms.MessageContent, 0, len(choice.ToolCalls))
	for _, tc := range choice.ToolCalls {
		handler, exists := agent.toolkit.HandlerForTool(tc.FunctionCall.Name)
		if !exists {
			toolCallResults = append(toolCallResults, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    "Error: tool does not exist in toolkit. All calls to this tool will fail",
					},
				},
			})
		} else {
			toolHandlerResponse, err := handler(ctx, tc.FunctionCall.Arguments)
			if err != nil {
				toolHandlerResponse = "Erroneous tool handler response. Txt=" + toolHandlerResponse + ". Err=" + err.Error()
			}
			toolCallResults = append(toolCallResults, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    toolHandlerResponse,
					},
				},
			})
		}
	}
	// TODO: handle  resp.Choices[0].Content in the new query as well?
	messages = append(messages, llms.MessageContent{ // TODO: do we really want to append on to the agent's current messages here?
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{llms.TextPart(choice.Content)},
	})
	messages = append(messages, toolCallResults...) // TODO: do we really want to append on to the agent's current messages here?
	// TODO: should a prompt for what to do with the results be provided?
	// TODO: add anything else on to the response that is necessary
	// TODO: pare down messages if needed
	finalResult, err := agent.promptRecursive(ctx, messages)
	if err != nil {
		return "recursive prompt error: " + finalResult, err
	}
	//agent.messages = messages // TODO: only keep the messages we want in long term!
	return finalResult, err
}

// Configuration toolkit
//var getConfigTool = llms.Tool{
//	Type: string(llms.ChatMessageTypeFunction), // "function"
//	Function: &llms.FunctionDefinition{
//		Name:        "Search",
//		Description: "A tool for searching the internet. Input should be a search query.",
//		Parameters: map[string]any{
//			"type": "object",
//			"properties": map[string]any{
//				"query": map[string]any{
//					"type":        "string",
//					"description": "The search query",
//				},
//			},
//			"required": []string{"query"},
//		},
//	},
//}

type LocalToolkit struct {
	m map[string]func(ctx context.Context, args string) (string, error)
	l []llms.Tool
}

func NewToolkit(tools ...CustomLocalTool) LocalToolkit {
	out := LocalToolkit{
		m: map[string]func(ctx context.Context, args string) (string, error){},
		l: []llms.Tool{},
	}
	return out.AddTools(tools...)
}
func (toolkit LocalToolkit) HandlerForTool(toolName string) (handler func(ctx context.Context, args string) (string, error), exists bool) {
	handler, exists = toolkit.m[toolName]
	return
}
func (toolkit LocalToolkit) AddTools(tools ...CustomLocalTool) LocalToolkit {
	for _, tool := range tools {
		toolkit.m[tool.Function.Name] = tool.handler
		toolkit.l = append(toolkit.l, tool.Tool)
	}
	return toolkit
}

var selectPathMockTool = llms.Tool{
	Type: "function",
	Function: &llms.FunctionDefinition{
		Name:        "SelectNextAgent",
		Description: "A tool selecting the next agent to pass through to.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agentName": map[string]any{
					"type":        "string",
					"description": "The name of the node or agent to pass through to",
				},
			},
			"required": []string{"agentName"},
		},
	},
}

func selectPathMockToolHandler(ctx context.Context, args string) (string, error) {
	var argsStruct struct {
		agentName string
	}
	err := json.Unmarshal([]byte(args), &argsStruct)
	if err != nil {
		return "", err
	}
	return argsStruct.agentName, nil
}

type CustomLocalTool struct {
	llms.Tool
	handler func(ctx context.Context, args string) (string, error)
}

// TODO: GET CONFIG TOOL???

var mockSelectNextAgentTool = CustomLocalTool{
	Tool:    selectPathMockTool,
	handler: selectPathMockToolHandler,
}

var setConfigValueTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "SetConfigValue",
			Description: "A tool for setting a configuration value",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "The key to set the value for",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "The value to set",
					},
					"configType": map[string]any{
						"type":        "string",
						"description": "The type of config to get the value for. Can be swarm, agent, or task. Default is agent",
					},
				},
				"required": []string{"key", "value"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Key        string  `json:"key"`
			Value      string  `json:"value"`
			ConfigType *string `json:"configType,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to ls"), err)
		}
		configType := Default(inp.ConfigType, "agent")
		cfg, err := getConfig(ctx, configType)
		if err != nil {
			return "", err
		}
		if cfg == nil {
			cfg = &Config{}
		}
		(*cfg)[inp.Key] = inp.Value
		ctx = setConfig(ctx, configType, cfg) // TODO: set this context on the agent???
		return "", nil
	},
}

func Default[T any](val *T, d T) T {
	if val == nil {
		return d
	}
	return *val
}

type Config map[string]string
type ConfigKey string

const (
	swarmConfigKey = "swarmConfig"
	agentConfigKey = "agentConfig"
	taskConfigKey  = "taskConfig"
)

func getConfig(ctx context.Context, configType string) (*Config, error) {
	switch configType {
	case swarmConfigKey:
		cfg, ok := ctx.Value(swarmConfigKey).(*Config)
		if !ok {
			return nil, nil
		}
		return cfg, nil
	case agentConfigKey:
		cfg, ok := ctx.Value(agentConfigKey).(*Config)
		if !ok {
			return nil, nil
		}
		return cfg, nil
	case taskConfigKey:
		cfg, ok := ctx.Value(taskConfigKey).(*Config)
		if !ok {
			return nil, nil
		}
		return cfg, nil
	default:
		return nil, errors.New("invalid configType")
	}
}
func setConfig(ctx context.Context, configType string, cfg *Config) context.Context {
	return context.WithValue(ctx, configType, cfg)
}

var getConfigValueTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "GetConfigValue",
			Description: "A tool for getting a configuration value",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "The key to get the value for",
					},
					"configType": map[string]any{
						"type":        "string",
						"description": "The type of config to get the value for. Can be swarm, agent, or task. Default is agent",
					},
				},
				"required": []string{"key"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Key        string  `json:"key"`
			ConfigType *string `json:"configType,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call"), err)
		}
		cfg, err := getConfig(ctx, Default(inp.ConfigType, "agent"))
		if err != nil {
			return "", err
		}
		if cfg == nil {
			return "", errors.New("no config")
		}
		val, ok := (*cfg)[inp.Key]
		if !ok {
			return "", errors.New("key not on config")
		}
		return val, nil
	},
}

//var searchInternetTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "Search",                                                             // TODO: FIX
//			Description: "A tool for searching the internet. Input should be a search query.", // TODO: FIX
//			Parameters: map[string]any{
//				"type": "object",
//				"properties": map[string]any{ // TODO: FIX
//					"query": map[string]any{
//						"type":        "string",
//						"description": "The search query",
//					},
//				},
//				"required": []string{"query"}, // TODO: FIX
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}

// Filesystem Tools
var listDirectoryTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "ListDirectory",
			Description: "A tool for listing files and directories within a directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path to the directory which will have its contents listed",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path string  `json:"path"`
			Only *string `json:"only,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to ls"), err)
		}
		entries, err := os.ReadDir(inp.Path)
		if err != nil {
			return "", errors.Join(errors.New("failed to read dir"), err)
		}
		// TODO: properly size outputs before doing anything else????

		filesInfo := []string{}
		dirsInfo := []string{}
		for _, entry := range entries {
			if entry.IsDir() {
				if inp.Only != nil && *inp.Only != "dirs" {
					continue
				}
				dirsInfo = append(dirsInfo, entry.Name())
			} else {
				filesInfo = append(filesInfo, entry.Name())
			}
		}
		var bs []byte
		if inp.Only != nil {
			switch *inp.Only {
			case "dirs":
				bs, err = json.Marshal(dirsInfo)
			case "files":
				bs, err = json.Marshal(filesInfo)
			default:
				return "", errors.Join(errors.New("invalid option for 'only' field"), err)
			}
		} else {
			bs, err = json.Marshal(struct {
				Dirs  []string `json:"dirs"`
				Files []string `json:"files"`
			}{Dirs: dirsInfo, Files: filesInfo})
		}
		if err != nil {
			return "", errors.Join(errors.New("failed to marshal ls info"), err)
		}
		return string(bs), nil
	},
}
var workingDirectory string

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic("failed to set working directory: " + err.Error())
	}
	workingDirectory = wd
}

type Path struct {
	Path string `json:"path"`
}

func (p Path) fullPath() string {
	return fullPathFor(p.Path)
}

var readFileTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "ReadFile",
			Description: "A tool for reading the contents of a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path to the file",
					},
					"startLine": map[string]any{
						"type":        "number",
						"description": "First line of the file to read, inclusive",
					},
					"endLine": map[string]any{
						"type":        "number",
						"description": "Last line of the file to read, inclusive",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path
			StartLine *int `json:"startLine,omitempty"`
			EndLine   *int `json:"endLine,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		fullPath := inp.fullPath()
		// Open the file
		file, err := os.Open(fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to open file: %w", err)
		}
		// Ensure the file is closed after the function finishes
		defer file.Close()

		var lines []string
		scanner := bufio.NewScanner(file)
		currentLine := 0

		// Iterate over each line
		hitStart := inp.StartLine != nil && *inp.StartLine > 0
		for scanner.Scan() {
			// Handle startline
			if !hitStart {
				if currentLine != *inp.StartLine {
					currentLine++
					continue
				}
				hitStart = true
			}

			currentLine++
			lines = append(lines, scanner.Text())
			// Stop scanning if the desired end line is hit
			if inp.EndLine != nil && *inp.EndLine == currentLine {
				break
			}
		}

		// Check for errors during scanning
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %w", err)
		}

		bs, err := json.Marshal(lines)
		if err != nil {
			return "", errors.Join(errors.New("failed to marshal readFile results"), err)
		}
		return string(bs), nil
	},
}
var readMultipleFilesTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "ReadFile",
			Description: "A tool for reading the contents of a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths": map[string]any{
						"type":        "array",
						"description": "An array of file paths.",
						"items": map[string]any{
							"type":        "string",
							"description": "A filepath",
						},
					},
				},
				"required": []string{"paths"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}

		results := make([]struct {
			Name    string
			Content []byte
		}, len(inp.Paths))
		for i, path := range inp.Paths {
			fullPath := fullPathFor(path)
			bs, err := os.ReadFile(fullPath)
			if err != nil {
				return "", fmt.Errorf("failed to open file: %w", err)
			}
			results[i] = struct {
				Name    string
				Content []byte
			}{Name: path, Content: bs}
		}
		bs, err := json.Marshal(results)
		if err != nil {
			return "", errors.New("failed to marshal results")
		}
		return string(bs), nil
	},
}
var writeFileTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "WriteFile",
			Description: "A tool for completely overwriting the contents of a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path to the file",
					},
					"data": map[string]any{
						"type":        "string",
						"description": "The file contents to be written",
					},
				},
				"required": []string{"path", "data"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path
			Data string `json:"data"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "failed to unmarshal tool call args", err
		}
		fullPath := inp.fullPath()
		if err := os.WriteFile(fullPath, []byte(inp.Data), 0644); err != nil {
			return "failed to write file", err
		}
		return "wrote file at " + inp.Path.Path, nil
	},
}
var createDirectoryTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "mkdir",
			Description: "A tool for creating a directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path of the directory",
					},
					"makeNested": map[string]any{
						"type":        "boolean",
						"description": "If true, will also make any parent directories that do not currently exist. Defaults to false.",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		//println("HIT CREATE DIR HANDLER with args: " + args)
		//return "successfully created directoy " + args, nil
		////return "HIT CREATE DIR HANDLER", errors.New("HIT CREATE DIR HANDLER")
		var inp struct {
			Path
			MakeNested bool `json:"makeNested"`
		}
		err := json.Unmarshal([]byte(args), &inp)
		if err != nil {
			return "failed to unmarshal tool call args", err
		}
		fullPath := inp.fullPath()
		dirPerm := os.ModePerm // TODO: perm ok?
		if inp.MakeNested {
			println("creating dir " + fullPath)
			err = os.MkdirAll(fullPath, dirPerm)
		} else {
			println("creating dir " + fullPath)
			err = os.Mkdir(fullPath, dirPerm)
		}
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				err = nil
			}
		}
		return "successfully created directory " + inp.Path.Path, err
	},
}
var moveFileTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "mv",
			Description: "A tool for moving a file from one location to another.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"src": map[string]any{
						"type":        "string",
						"description": "The original location of the file",
					},
					"dst": map[string]any{
						"type":        "string",
						"description": "The destination location of the file",
					},
				},
				"required": []string{"src", "dst"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		}
		err := json.Unmarshal([]byte(args), &inp)
		if err != nil {
			return "failed to unmarshal tool call args", err
		}
		srcPath := fullPathFor(inp.Src)
		dstPath := fullPathFor(inp.Dst)
		if err = os.Rename(srcPath, dstPath); err == nil {
			return "successfully moved file " + inp.Src + " to " + inp.Dst, nil
		}
		src, err := os.OpenFile(srcPath, os.O_RDONLY, 0644)
		if err != nil {
			return "failed to open new file", err
		}
		dst, err := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644) // TODO: trunc ok?
		if err != nil {
			return "failed to open new file", err
		}
		_, err = io.Copy(dst, src)
		if err != nil {
			return "failed to copy file", err
		}
		return "successfully moved file " + inp.Src + " to " + inp.Dst, os.Remove(inp.Src)
	},
}
var searchFilesTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "SearchFiles",
			Description: "Find files by name using case-insensitive substring matching",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{ // TODO: FIX
					"root": map[string]any{
						"type":        "string",
						"description": "The directory to search within",
					},
					"searchTerm": map[string]any{
						"type":        "string",
						"description": "The case-insensitive filename substring to search for",
					},
					// TODO: top level only?
				},
				"required": []string{"searchTerm"}, // TODO: no root?
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		// TODO: THIS!
		return "", errors.New("NOT IMPLEMENTED")
	},
}

func fullPathFor(agentFilePath string) string {
	return workingDirectory + "/" + agentFilePath
}

var getFileInfoTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "GetFileInfo",
			Description: "Retrieve detailed metadata about a file or directory",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The file or directory path",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path
		}
		err := json.Unmarshal([]byte(args), &inp)
		if err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		fileInfo, err := os.Stat(inp.fullPath())
		if err != nil {
			return "", errors.Join(errors.New("failed to stat file"), err)
		}
		bs, err := json.Marshal(fileInfo) // TODO: is just using fileInfo ok?
		if err != nil {
			return "", errors.Join(errors.New("failed to marshal fileInfo"), err)
		}
		return string(bs), nil
	},
}

/*
read_file - Read the contents of a file with optional start_line and end_line parameters for paging
read_multiple_files - Read the contents of multiple files simultaneously
write_file - Completely replace file contents
create_directory - Create a new directory or ensure a directory exists
list_directory - Get a detailed listing of all files and directories in a specified path
move_file - Move or rename files and directories
search_files - Find files by name using case-insensitive substring matching
get_file_info - Retrieve detailed metadata about a file or directory
*/

// Terminal Tools:
var executeClientTerminalCommandTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "ExecuteClientTerminalCommand",
			Description: "Execute a terminal command with a timeout client-side",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cmd": map[string]any{
						"type":        "string",
						"description": "The command to execute client-side",
					},
					"timeoutSecs": map[string]any{ // TODO; DEFAULT???
						"type":        "number", // TODO: ok?
						"description": "Timeout in seconds",
					},
				},
				"required": []string{"cmd"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Cmd         string `json:"cmd"`
			TimeoutSecs *int   `json:"timeoutSecs"`
		}
		err := json.Unmarshal([]byte(args), &inp)
		if err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		// Define a timeout duration
		timeout := time.Duration(Default(inp.TimeoutSecs, 10)) * time.Second

		// Create a context with a timeout
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel() // Release resources associated with the context when the function returns

		// The command to run. We use "sleep 5" as an example of a long-running command.
		// To run it in a shell, we use "sh -c".
		// TODO: cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 5")

		// Run the command and wait for it to finish or the context to time out
		output, err := exec.CommandContext(ctx, "sh", inp.Cmd).
			CombinedOutput()

		// Check the error to see if a timeout occurred
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "command timed out", err
			}
			return "Command finished with error", err
		}
		return string(output), nil
	},
}

//	var executeServerTerminalCommandTool = CustomLocalTool{
//		Tool: llms.Tool{
//			Type: "function",
//			Function: &llms.FunctionDefinition{
//				Name:        "ExecuteClientTerminalCommand",
//				Description: "Execute a terminal command with timeout server-side",
//				Parameters: map[string]any{
//					"type": "object",
//					"properties": map[string]any{
//						"cmd": map[string]any{
//							"type":        "string",
//							"description": "The command to execute",
//						},
//						"timeoutSecs": map[string]any{ // TODO; DEFAULT???
//							"type":        "number", // TODO: ok?
//							"description": "Timeout in seconds",
//						},
//					},
//					"required": []string{"cmd"},
//				},
//			},
//		},
//		handler: func(ctx context.Context, args string) (string, error) {
//			// TODO: THIS!
//			return "", errors.New("NOT IMPLEMENTED")
//		},
//	}
//var readTerminalOutputTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "ReadTerminalOutput",
//			Description: "Read new output from a running terminal session",
//			Parameters: map[string]any{
//				"type":       "object",
//				"properties": map[string]any{ // TODO: client/server?
//					//"cmd": map[string]any{
//					//	"type":        "string",
//					//	"description": "The command to execute",
//					//},
//					//"timeoutSecs": map[string]any{ // TODO; DEFAULT???
//					//	"type":        "number", // TODO: ok?
//					//	"description": "Timeout in seconds",
//					//},
//				},
//				"required": []string{},
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}
//var forceTerminateTerminalSessionTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "TerminateTerminalSession",
//			Description: "Force terminate a running terminal session",
//			Parameters: map[string]any{
//				"type": "object",
//				"properties": map[string]any{ // TODO: client/server?
//					"id": map[string]any{
//						"type":        "string",
//						"description": "Terminal session ID to terminate",
//					},
//					//"cmd": map[string]any{
//
//					//},
//					//"timeoutSecs": map[string]any{ // TODO; DEFAULT???
//					//	"type":        "number", // TODO: ok?
//					//	"description": "Timeout in seconds",
//					//},
//				},
//				"required": []string{"id"},
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}
//var listTerminalSessionsTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "ListTerminalSessions",
//			Description: "List all active terminal sessions",
//			Parameters: map[string]any{
//				"type": "object",
//				"properties": map[string]any{
//					"location": map[string]any{
//						"type":        "string",
//						"description": "location to list the terminal sessions on. client or server",
//					},
//				},
//				"required": []string{"location"},
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}

//execute_command - Execute a terminal command with timeout
//execute_in_terminal - Execute a command in the terminal (client-side execution)
//read_output - Read new output from a running terminal session
//force_terminate - Force terminate a running terminal session
//list_sessions - List all active terminal sessions

// Search Tools:
// search_code - Search for text/code patterns within file contents using ripgrep
var searchCodeTool = CustomLocalTool{ // TODO; DO THIS!
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "SearchCode",
			Description: "Search for text/code patterns within file contents using ripgrep",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "the path of the file to search",
					},
					"pattern": map[string]any{
						"type":        "string",
						"description": "the pattern to search for", // TODO: REGEX?
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		// TODO: THIS!
		return "", errors.New("NOT IMPLEMENTED")
	},
}

// Edit Tools:
// edit_block - Apply surgical text replacements to files
var replaceCodeBlockTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "EditBlock",
			Description: "Apply surgical text replacements to files",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "the path of the file to replace a block within",
					},
					"replace": map[string]any{
						"type":        "string",
						"description": "the string to replace", // TODO: REGEX?
					},
					"replacement": map[string]any{
						"type":        "string",
						"description": "the value to replace the selected string with", // TODO: REGEX?
					},
					"numToReplace": map[string]any{
						"type":        "number",
						"description": "The number of instances of the replace string to be replaced",
					},
				},
				"required": []string{"path", "replace", "replacement"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path         string `json:"path"`
			Replace      string `json:"replace"`
			Replacement  string `json:"replacement"`
			NumToReplace *int   `json:"numToReplace,omitempty"`
		}
		err := json.Unmarshal([]byte(args), &inp)
		if err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		err = replaceTextInFile(inp.Path, inp.Replace, inp.Replacement, inp.NumToReplace)
		if err != nil {
			if errors.Is(err, errContentUnchanged) {
				return "file content not changed", err
			}
		}
		return "", err
	},
}
var errContentUnchanged = errors.New("file content is unchanged")

func replaceTextInFile(filePath, oldText, newText string, numToChange *int) error {
	// Read the entire file content
	content, err := os.ReadFile(filePath) // Use os.ReadFile (Go 1.16+)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	modifiedContent := strings.Replace(string(content), oldText, newText, Default(numToChange, -1))

	// Check if any replacement actually happened (optional optimization)
	if modifiedContent == string(content) {
		return errContentUnchanged
	}
	err = os.WriteFile(filePath, []byte(modifiedContent), 0644)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Successfully replaced all occurrences of '%s' with '%s' in %s\n", oldText, newText, filePath)
	return nil
}

// precise_edit - Precisely edit file content based on start and end line numbers
var preciseEditTool = CustomLocalTool{
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "PreciseEdit",
			Description: "Precisely edits file content based on start and end line numbers (inclusive)",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path to the file",
					},
					"data": map[string]any{
						"type":        "string",
						"description": "The file contents to be written",
					},
					"startLine": map[string]any{
						"type":        "number",
						"description": "The first line to be overwritten",
					},
					"endLine": map[string]any{
						"type":        "number",
						"description": "The last line to be overwritten",
					},
				},
				"required": []string{"path", "data", "startLine", "endLine"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path      string `json:"path"`
			Data      string `json:"data"`
			StartLine int    `json:"startLine,omitempty"`
			EndLine   int    `json:"endLine,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		return "", replaceFileLines(inp.Path, inp.StartLine, inp.EndLine, inp.Data)
	},
}

func replaceFileLines(path string, startLine, endLine int, contents string) error {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create temporary output file
	workingDir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(workingDir, "preciseEdit")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	writer := bufio.NewWriter(tempFile)

	lineCount := 1
	for scanner.Scan() {
		if lineCount == startLine {
			if _, err = fmt.Fprintln(writer, contents); err != nil {
				writer.Flush()
				file.Close()
				tempFile.Close()
				return err
			}
			continue
		}
		// (1-indexed) // TODO: ew?
		// Skip lines we want to replace
		if lineCount >= startLine && lineCount <= endLine {
			continue
		}
		if _, err = fmt.Fprintln(writer, scanner.Text()); err != nil {
			writer.Flush()
			file.Close()
			tempFile.Close()
			return err
		}
		lineCount++
	}

	writer.Flush()
	file.Close()
	tempFile.Close()

	// Replace original file with temporary file
	return os.Rename(tempFile.Name(), path)
}

// Process Tools:
//var listProcessesTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "ListProcesses",
//			Description: "List all running processes",
//			Parameters: map[string]any{
//				"type":       "object",
//				"properties": map[string]any{ // TODO: client/server?
//				},
//				"required": []string{},
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}
//var killProcessTool = CustomLocalTool{
//	Tool: llms.Tool{
//		Type: "function",
//		Function: &llms.FunctionDefinition{
//			Name:        "Terminate a running process by PID",
//			Description: "List all running processes",
//			Parameters: map[string]any{
//				"type": "object",
//				"properties": map[string]any{
//					"pid": map[string]any{
//						"type":        "number",
//						"description": "the PID to terminate",
//					},
//				},
//				"required": []string{"pid"},
//			},
//		},
//	},
//	handler: func(ctx context.Context, args string) (string, error) {
//		// TODO: THIS!
//		return "", errors.New("NOT IMPLEMENTED")
//	},
//}
//
//// TODO: a tool to show the entire contents (tree) of a directory??
//
//// TODO: Filesystem Tools
//// TODO: Search Tools
//// TODO: Edit Tools
//// TODO: Terminal Tools
//// TODO: Process Tools
//// TODO: git toolkit?
//// TODO: testing tools?
//
///*
// * Configuration Tools:
//get_config - Get the complete server configuration as JSON
//set_config_value - Set a specific configuration value by key

//var exampleTool = llms.Tool{
//	Type: "function",
//	Function: &llms.FunctionDefinition{
//		Name:        "Search",
//		Description: "A tool for searching the internet. Input should be a search query.",
//		Parameters: map[string]any{
//			"type": "object",
//			"properties": map[string]any{
//				"query": map[string]any{
//					"type":        "string",
//					"description": "The search query",
//				},
//			},
//			"required": []string{"query"},
//		},
//	},
//}

// Git Tools
var createWorktreeTool = CustomLocalTool{ // Runs git worktree add <path> -b <branch_name>.
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "CreateGitWorktree",
			Description: "Creates a git worktree",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The worktree root directory",
					},
					"branch": map[string]any{
						"type":        "string",
						"description": "The branch to create the worktree for. Defaults to main",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path   string  `json:"path"`
			Branch *string `json:"branch,omitempty"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		if inp.Branch == nil {
			inp.Branch = Pointer("main")
		}
		bs, err := exec.Command("git", "worktree add "+inp.Path+" -b "+*inp.Branch).CombinedOutput()
		if err != nil {
			return "", errors.Join(errors.New("worktree add failure"), err)
		}
		return string(bs), nil
	},
}

func Pointer[T any](val T) *T {
	return &val
}

var removeWorktreeTool = CustomLocalTool{ // Runs git worktree add <path> -b <branch_name>.
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "RemoveGitWorktree",
			Description: "Removes a git worktree",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The worktree root directory",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}

		bs, err := exec.Command("git", "worktree remove "+inp.Path).CombinedOutput()
		if err != nil {
			return "", errors.Join(errors.New("worktree add failure"), err)
		}
		return string(bs), nil
	},
}
var runCommandInWorktreeTool = CustomLocalTool{ //  Executes commands (e.g., git pull, git commit, npm install, pytest) within the specific worktree's directory.
	Tool: llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "RunWorktreeCommand",
			Description: "Runs a command on a worktree",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The worktree root directory",
					},
					"cmd": map[string]any{
						"type":        "string",
						"description": "The command to run",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	handler: func(ctx context.Context, args string) (string, error) {
		var inp struct {
			Path string `json:"path"`
			Cmd  string `json:"cmd"`
		}
		if err := json.Unmarshal([]byte(args), &inp); err != nil {
			return "", errors.Join(errors.New("failed to unmarshal tool call args"), err)
		}
		bs, err := runCmdFromDir(inp.Path, inp.Cmd)
		if err != nil {
			return "", errors.Join(errors.New("worktree command failure"), err)
		}
		return string(bs), errors.New("NOT IMPLEMENTED")
	},
}

// TODO: how to merge and fix conflicts?

func runCmdFromDir(dir string, cmd string) ([]byte, error) {

	// Create the command
	cmdToRun := exec.Command(cmd)
	//cmdToRun := exec.Command(cmd, args...)
	// Set the working directory
	cmdToRun.Dir = dir

	// Run the command and capture the output (stdout and stderr combined)
	return cmdToRun.CombinedOutput()
}
