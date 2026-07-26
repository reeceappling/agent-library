package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/reeceappling/agent-library/examples/agents/poolside"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms"
	"log"
	"os"
)

// TODO: REFER TO THIS! https://medium.com/@alma.tuck/how-to-build-your-own-mcp-vibe-coding-server-in-go-using-gomcp-c80ad2e2377c
/*
 * Configuration Tools:
get_config - Get the complete server configuration as JSON
set_config_value - Set a specific configuration value by key

 * Filesystem Tools:
read_file - Read the contents of a file with optional start_line and end_line parameters for paging
read_multiple_files - Read the contents of multiple files simultaneously
write_file - Completely replace file contents
create_directory - Create a new directory or ensure a directory exists
list_directory - Get a detailed listing of all files and directories in a specified path
move_file - Move or rename files and directories
search_files - Find files by name using case-insensitive substring matching
get_file_info - Retrieve detailed metadata about a file or directory

 * Search Tools:
search_code - Search for text/code patterns within file contents using ripgrep

 * Edit Tools:
edit_block - Apply surgical text replacements to files
precise_edit - Precisely edit file content based on start and end line numbers

 * Terminal Tools:
execute_command - Execute a terminal command with timeout
execute_in_terminal - Execute a command in the terminal (client-side execution)
read_output - Read new output from a running terminal session
force_terminate - Force terminate a running terminal session
list_sessions - List all active terminal sessions

 * Process Tools:
list_processes - List all running processes
kill_process - Terminate a running process by PID
*/

// TODO: CONTEXT WINDOW == 128k tokens for the totality of the trajectory input + between + output

//var (
//	maxTokens = 128000
//	maxOutputTokens = 28000
//	maxInputTokens = 90000
//	workingTokens = maxTokens - (maxOutputTokens + maxInputTokens)
//	bytesPerTokenConservative = 3
//	maxOutputBytes = maxOutputTokens * bytesPerTokenConservative
//	maxInputBytes = maxInputTokens * bytesPerTokenConservative
//	maxWorkingBytes = workingTokens * bytesPerTokenConservative
//)

// SearchTool is a mock tool for demonstration purposes.
// In a real application, this would interact with an external search engine.
type SearchTool struct{}

func (t SearchTool) Name() string {
	return "Search"
}

func (t SearchTool) Description() string {
	return "A tool for searching the internet. Input should be a search query."
}

func (t SearchTool) Call(ctx context.Context, input string) (string, error) {
	// Mocking a search result
	return fmt.Sprintf("The search result for '%s' is: The Go programming language is popular for backend development.", input), nil
}

// SearchTool is a mock tool for demonstration purposes.
// In a real application, this would interact with an external search engine.
type AskUserTool struct{}

func (t AskUserTool) Name() string {
	return "AskUserAQuestion"
}

func (t AskUserTool) Description() string {
	return "A tool for asking the user a question, either for clarification or for extra information."
}

func (t AskUserTool) Call(ctx context.Context, input string) (string, error) {
	println(input)
	// Create a scanner that reads from os.Stdin
	scanner := bufio.NewScanner(os.Stdin)

	// Loop through each line of input
	for scanner.Scan() {
		userInput := scanner.Text() // Get the line as a string (without the newline character)
		return userInput, nil
	}

	return "", scanner.Err()
}

type LLMAgent struct {
	model    llms.Model
	messages []llms.MessageContent
	toolkit  LocalToolkit
}

func NewLLMAgent(apiKey string, tools LocalToolkit) *LLMAgent {
	llm, err := poolside.New(poolside.WithToken(apiKey))
	if err != nil {
		panic(err)
	}
	return &LLMAgent{
		model:    llm,
		messages: []llms.MessageContent{},
		toolkit:  tools,
		//grafana: NewGrafanaTool()
	}
}

type CustomState struct {
	target   string
	goal     string // TODO: ok?
	messages []llms.MessageContent
}

func main() {
	// 1. Locate your mcp.json configuration file
	//configPath := filepath.Join(".", "mcp.json") // TODO: FIX THIS!
	// TODO: BANKING/CARD MCP https://banking-agent.robinhood.com/mcp/banking
	/*
		monitors one signal in particular: the trading activity of politicians, CEOs, and executives. These people often hold information the rest of us do not, so the agent watches their public moves and weighs them when picking stocks.
	*/
	// RH MCP can ask portfolio value, buying power, account information, place orders of different types
	/* TODO:
	If the market drops more than x amount over the course of Y, then buy in
	If the market goes up over 100% in a week, get out?
	"Look at my portfolio and tell me what risks I am exposed to"
	"Why is the market up today?"
	Currently can long stocks and maybe options
	robinhood.com/us/en/support/articles/trading-with-your-agent
	Tools:
		Account, portfolio, and other tools
			get_accounts	View all your Robinhood accounts
			get_portfolio	Get a snapshot of your portfolio—including total value—plus values by asset class and real-time buying power
			search	Find a company name or partial name to a ticker
		Watchlist tools:
			get_watchlists	List user’s watchlists
			get_watchlist_items	List symbols in a specific watchlist
			get_option_watchlist	Load an options watchlist
			get_popular_watchlists	Discover Robinhood lists (like “100 most popular”)
			create_watchlist	Make a new custom watchlist.
			update_watchlist	Rename or update a watchlist’s name description
			follow_list	Follow a Robinhood list
			unfollow_list	Stop following a Robinhood list
			add_to_watchlist	Add stocks, crypto, or indexes to a watchlist
			remove_from_watchlist	Remove stocks, crypto, or indexes from a watchlist
			add_option_to_watchlist	Add an options contract to a watchlist
			remove_option_from_watchlist	Remove an options contract from a watchlist
		Market data tools
			get_equity-historicals	Get OHLCV price bars across a time range
			get_indexes	Look up market indexes by symbol
			get_indexes_quotes	Get real-time index values
		Equities tools
			get_equity_positions	View open equity positions with quantity and cost basis
			get_equity_quotes	Get real-time equity quotes and prior close for up to 20 symbols
			get_equity_orders	Get equity order status history
			get_equity_tradability	Check if a symbol can be traded and find out if it can be traded fractionally
			review_equity_order	Simulate an equity order and get pre-trade warnings
			place_equity_order	Place an equity order
			cancel_equity_order	Cancel an open equity order
		Options tools (may not be available to everyone)
			get_option_chains	Load option chains
			get_option_instruments	Load option contracts filtered by expiry, strike, or type
			get_option_quotes	Get real-time quotes for option contracts
			get_option_positions	View open or closed options positions
			get_option_orders	Get options order history
			review_option_order	Simulate an options order with pre-trade alerts
			cancel_option_order	Cancel an open options order
			place_option_order	Place a real options order

	I want to track politicians trades, with a preference for getting in on those which are down in the short term
	I want to run the wheel on ?????
	HOW TO STORE INFO ON TRADES?
	*/
	// Set your OpenAI API key in your environment variables
	// export OPENAI_API_KEY="your-api-key"
	apiKey := os.Getenv("POOLSIDE_API_KEY")
	//query := "What is 2+2? respond with only the number and nothing else. Do not include a line ending at the end of the response string either"
	ctx := context.Background()

	g := graph.NewStateGraph[*CustomState]() // TODO: change this?
	managementAgentToolkit := NewToolkit()   // TODO: ?????
	codingAgentToolkit := NewToolkit(createDirectoryTool, writeFileTool, moveFileTool /*, mockSelectNextAgentTool*/)
	//codingAgentToolkit.AddTools(createDirectoryTool)
	// TODO: NewLocalToolkit?

	llmAgent := NewLLMAgent(apiKey, managementAgentToolkit)
	codingAgent := NewLLMAgent(apiKey, codingAgentToolkit)
	//g.AddNode("entrypoint", "description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
	//	// TODO: Step memory, long-term memory,
	//	// TODO: state is short term, and agent mem is long term
	//	resp, err := llmAgent.Prompt(ctx, state.messages)
	//	if err != nil {
	//		// TODO: add response to agent messages (long term)????
	//		return state, err
	//	}
	//	state.messages = append(state.messages,
	//		llms.TextParts(llms.ChatMessageTypeAI, resp),
	//	)
	//	return state, nil
	//})
	g.AddNode("entrypoint", "description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
		// TODO: Step memory, long-term memory,
		// TODO: state is short term, and agent mem is long term
		resp, err := llmAgent.Prompt(ctx, state.messages)
		if err != nil {
			// TODO: add response to agent messages (long term)????
			return state, err
		}
		println("response from entrypoint agent...", resp)
		switch resp {
		case "node1":
			state.target = "node1"
		case "node2":
			state.target = "node2"
		default:
			println(resp, "INVALID OPTION!") // TODO: remove any trailing newlines!
			state.target = graph.END
		}
		state.messages = append(state.messages,
			llms.TextParts(llms.ChatMessageTypeAI, "entrypoint selected "+resp),
		)
		return state, nil
	})

	//addNode := func(name string) {
	//	g.AddNode(name, name+"_description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
	//		prompt := "Use the tools at your disposal to make a directory named " + name + "." //". Once the directory has been successfully created, respond with the name of the directory you just made, and only that name."
	//		msgs := []llms.MessageContent{
	//			//{
	//			//	Role: llms.ChatMessageTypeSystem,
	//			//	Parts: []llms.ContentPart{llms.TextPart("Try to call a tool, any tool, I dont care what the name is or if it exists. I just want to see a tool call on the response to this prompt."),
	//			//		llms.TextPart("Last time, you responded with the json to call a tool in your response message. That is not what I want. I want the response to be parseable such that langgraph can call the appropriate tool."),
	//			//	},
	//			//},
	//			//{
	//			//	Role:  llms.ChatMessageTypeSystem,
	//			//	Parts: []llms.ContentPart{llms.TextPart("List all the tools and functions available to you as an ai agent running in langgraph. I have given you two tools, what are their names?")},
	//			//},
	//			//{
	//			//	Role:  llms.ChatMessageTypeSystem,
	//			//	Parts: []llms.ContentPart{llms.TextPart("you are an autonomous senior software engineer. You must resolve any prompts utilizing the tools and functions you have available to you. " + prompt)},
	//			//},
	//			{
	//				Role:  llms.ChatMessageTypeSystem,
	//				Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
	//			},
	//		}
	//		resp, err := llmAgent.Prompt(ctx, msgs)
	//		if err != nil {
	//			return state, err
	//		}
	//		println(name + " was prompted. Result == " + resp) // TODO; del
	//		state.messages = append(state.messages, llms.MessageContent{
	//			Role:  llms.ChatMessageTypeAI,
	//			Parts: []llms.ContentPart{llms.TextContent{Text: resp}},
	//		})
	//		//state.messages = append(state.messages,
	//		//	llms.TextParts(llms.ChatMessageTypeAI, resp),
	//		//)
	//		println(name + " UTILIZED!")
	//		return state, nil
	//	})
	//}
	//addNode("node1")
	//addNode("node2")
	g.AddNode("node1", "node1_description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
		prompt := "Use the tools at your disposal to make a directory named node1." //". Once the directory has been successfully created, respond with the name of the directory you just made, and only that name."
		msgs := []llms.MessageContent{
			{
				Role:  llms.ChatMessageTypeSystem,
				Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
			},
		}
		resp, err := codingAgent.Prompt(ctx, msgs)
		if err != nil {
			return state, err
		}
		println("node1 was prompted. Result == " + resp) // TODO; del
		state.messages = append(state.messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{llms.TextContent{Text: resp}},
		})
		//state.messages = append(state.messages,
		//	llms.TextParts(llms.ChatMessageTypeAI, resp),
		//)
		println("node1 UTILIZED!")
		return state, nil
	})
	g.AddNode("node2", "node2_description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
		prompt := "Use the tools at your disposal to make a file named node2/node2.txt with the contents 'agent 2 wrote this!', then move that file to the node1 directory" //". Once the directory has been successfully created, respond with the name of the directory you just made, and only that name."
		msgs := []llms.MessageContent{
			{
				Role:  llms.ChatMessageTypeSystem,
				Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
			},
		}
		resp, err := codingAgent.Prompt(ctx, msgs)
		if err != nil {
			return state, err
		}
		println("node2 was prompted. Result == " + resp) // TODO; del
		state.messages = append(state.messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{llms.TextContent{Text: resp}},
		})
		//state.messages = append(state.messages,
		//	llms.TextParts(llms.ChatMessageTypeAI, resp),
		//)
		println("node2 UTILIZED!")
		return state, nil
	})
	//g.AddNode("node1", "node1_description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
	//	// TODO: Step memory, long-term memory,
	//	// TODO: state is short term, and agent mem is long term
	//	prompt := "make a directory named node1. If successful, your response should just be 'node1'"
	//	resp, err := llmAgent.Prompt(ctx, []llms.MessageContent{{
	//		Role:  llms.ChatMessageTypeSystem,
	//		Parts: []llms.ContentPart{llms.TextContent{Text: prompt}},
	//	}})
	//	if err != nil {
	//		// TODO: add response to agent messages (long term)????
	//		return state, err
	//	}
	//	state.messages = append(state.messages,llms.MessageContent{
	//		Role:  llms.ChatMessageTypeAI,
	//		Parts: []llms.ContentPart{llms.TextContent{Text: resp}},
	//	})
	//	//state.messages = append(state.messages,
	//	//	llms.TextParts(llms.ChatMessageTypeAI, resp),
	//	//)
	//	println("NODE 1 UTILIZED!")
	//	return state, nil
	//})
	//g.AddNode("node2", "node2_description_here", func(ctx context.Context, state *CustomState) (*CustomState, error) {
	//	// TODO: Step memory, long-term memory,
	//	// TODO: state is short term, and agent mem is long term
	//	resp, err := llmAgent.Prompt(ctx, state.messages)
	//	if err != nil {
	//		// TODO: add response to agent messages (long term)????
	//		return state, err
	//	}
	//	//state.messages = append(state.messages,
	//	//	llms.TextParts(llms.ChatMessageTypeAI, resp),
	//	//)
	//	println("NODE 2 UTILIZED")
	//	return state, nil
	//})
	g.AddNode(graph.END, "end-description", func(ctx context.Context, state *CustomState) (*CustomState, error) {
		println("END NODE USED")
		return state, nil
	})
	g.AddConditionalEdge("entrypoint", func(ctx context.Context, state *CustomState) string {
		switch state.target { // TODO: can be node 1 or two ONLY
		case "entrypoint":
			return graph.END
		case "node1":
			return state.target
		case "node2":
			return state.target
		}
		return graph.END
	})
	g.SetEntryPoint("entrypoint")
	g.AddEdge("node1", graph.END)
	g.AddEdge("node2", graph.END)
	g.AddEdge("entrypoint", graph.END)

	runnable, err := g.Compile()
	if err != nil {
		panic(err)
	}

	// Let's run it!
	for _, agentNum := range []int{1, 2} {
		println(fmt.Sprintf("Running agent %d", agentNum)) // TODO: del
		prompt := fmt.Sprintf(`select the agent named "node%d".Respond ONLY with the name of the agent to call`, agentNum)
		res, err := runnable.Invoke(ctx, &CustomState{
			target: "startQuery", // TODO: fix
			goal:   "???",        // TODO: fix
			messages: []llms.MessageContent{
				llms.TextParts(llms.ChatMessageTypeHuman, prompt),
			},
		})
		if err != nil {
			log.Fatal(err.Error())
		}

		fmt.Println(res)
	}

	// Output:
	// [{human [{What is 1 + 1?}]} {ai [{1 + 1 equals 2.}]}]

	// Initialize the LLM (Large Language Model)

	//	agentTools := []tools.Tool{
	//		SearchTool{}, // TODO: mock search tool
	//		AskUserTool{}, // TODO: mock search tool
	//	} // TODO: add codingAgentToolkit!
	//	llmsTools := []llms.Tool{
	//		{
	//			Type:     "function",
	//			Function: &llms.FunctionDefinition{
	//				Name:        "Search",
	//				Description: "A tool for searching the internet. Input should be a search query.",
	//				Parameters: map[string]any{
	//					"type": "object",
	//					"properties": map[string]any{
	//						"query": map[string]any{
	//							"type":        "string",
	//							"description": "The search query",
	//						},
	//					},
	//					"required": []string{"query"},
	//				},
	//			},
	//		},
	//		{
	//			Type:     "function",
	//			Function: &llms.FunctionDefinition{
	//				Name:        "AskUserAQuestion",
	//				Description: "Ask the user a question, either for clarification or for extra information.",
	//				Parameters: map[string]any{
	//					"type": "object",
	//					"properties": map[string]any{
	//						"question": map[string]any{
	//							"type":        "string",
	//							"description": "The question",
	//						},
	//					},
	//					"required": []string{"question"},
	//				},
	//			},
	//		},
	//	} // TODO: add codingAgentToolkit!
	//	//toolCallOptions := agents.WithMaxIterations(1)
	//	agentOptions := []agents.Option{
	//		agents.WithMaxIterations(1),
	//	} // TODO: add options!
	//
	//
	//
	//	resp, err := llmAgent.model.GenerateContent(ctx, llmAgent.messages, llms.WithTools(llmsTools))
	//	if err != nil {
	//		panic(err)
	//	}
	//	choice := resp.Choices[0]
	//	content := choice.Content
	//
	//	toolChoiceResponse := extractContent(content)
	//	fmt.Println("Tool Choice Response: ", toolChoiceResponse)
	//
	//	if len(choice.ToolCalls) == 0 {
	//		return "I’m sorry, but I’m not able to help with that request.", nil
	//	}
	//
	//	for _, tc := range choice.ToolCalls {
	//		fmt.Println("LLM chooses tool: ", tc.FunctionCall.Name)
	//
	//		// this is where we take action based on what tool the LLM decideds to use
	//		switch tc.FunctionCall.Name {
	//		case "FindClustersFromPod":
	//			var args struct {
	//				PodName string `json:"podName"`
	//			}
	//			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
	//				return "", fmt.Errorf("failed to Unmarshal argument err: %v", err)
	//			}
	//			//call the actual tool
	//			clusterNames, err := agent.FindClustersFromPod(ctx, args.PodName)
	//			if err != nil {
	//				return "", fmt.Errorf("failed to find FindClustersFromPod err: %v", err)
	//			}
	//
	//			// append the response of the tool call back with ChatMessageTypeTool
	//			clusterNamesResponse := llms.MessageContent{
	//				Role: llms.ChatMessageTypeTool,
	//				Parts: []llms.ContentPart{
	//					llms.ToolCallResponse{
	//						ToolCallID: tc.ID,
	//						Name:       tc.FunctionCall.Name,
	//						Content:    clusterNames,
	//					},
	//				},
	//			}
	//			agent.messages = append(agent.messages, clusterNamesResponse)
	//		case "FindClustersFromAppNameOrNamespace":
	//			<redacted as it's similar
	//		default:
	//			return "", fmt.Errorf("wrong tool selection")
	//		}
	//	}
	//}
	//
	//	//agent := agents.NewOneShotAgent(model, agentTools, agentOptions...)
	//	//resp, err := agent.model.GenerateContent(ctx, agent.messages, llms.WithTools(agentTools))
	//	//if err != nil {
	//	//	return "", fmt.Errorf("failed to get LLM response: %v", err)
	//	//}
	//
	//	//TODO: need access to model.GenerateContent()
	//	agent.Chain.
	//	// TODO: mrkl agent should
	//
	//	executor := agents.NewExecutor(agent, agentOptions...)
	//	if executor == nil {
	//		panic("nil executor")
	//	}
	//	question := "What is the Go programming language known for?"
	//	fmt.Printf("Running agent with prompt: %s\n", question)
	//	chainCallOpts := []chains.ChainCallOption{}
	//	// TODO: REFER TO https://medium.com/learnings-from-the-paas/developing-my-first-sre-helper-llm-agent-using-langchaingo-63f4201636f5
	//	chains.Call(ctx, executor, nil, chainCallOpts...)
	//	result, err := chains.Run(ctx, executor, question, chainCallOpts...) // TODO: NOT WORKING
	//	if err != nil {
	//		panic(err)
	//	}
	//	if !strings.Contains(result, "The Go programming language is popular") {
	//		panic("result was " + result)
	//	}
	//	println("results: ", result)
	//
	//	//response, err := model.GenerateContent(ctx, []llms.MessageContent{
	//	//	{
	//	//		Role:  llms.ChatMessageTypeHuman,
	//	//		Parts: []llms.ContentPart{llms.TextPart("The number associated with this message is 4")},
	//	//	}, {
	//	//		Role:  llms.ChatMessageTypeHuman,
	//	//		Parts: []llms.ContentPart{llms.TextPart("What is the number from the first message, multiplied by 5? Respond only with the number, and nothing else")},
	//	//	},
	//	//	//{
	//	//	//Role:  llms.ChatMessageTypeHuman,
	//	//	//Parts: []llms.ContentPart{
	//	//	//	//llms.BinaryPart("mimeType", []byte{}),
	//	//	//	//llms.ImageURLPart("google.com/banner.png"),
	//	//	//	//llms.ImageURLWithDetailPart("google.com/banner.png","google homepage image"),
	//	//	//	//llms.CachedContent{ // TODO: use with CacheControl
	//	//	//	//	ContentPart:  llms.BinaryPart("mimeType", []byte{}),
	//	//	//	//	CacheControl: &llms.CacheControl{
	//	//	//	//		Type:     "type-of-caching (ex: ephemeral)",
	//	//	//	//		Duration: 5 * time.Minute, // cache duration
	//	//	//	//	},
	//	//	//	//},
	//	//	//	//llms.ToolCall{
	//	//	//	//	ID:           "toolCallId",
	//	//	//	//	Type:         "function",
	//	//	//	//	FunctionCall: &llms.FunctionCall{
	//	//	//	//		Name:      "functionName",
	//	//	//	//		Arguments: `{"argName1":"arg1","argName2":"arg2"}`,
	//	//	//	//	},
	//	//	//	//},
	//	//	//	//llms.ToolCallResponse{
	//	//	//	//	ToolCallID: "toolCallId",
	//	//	//	//	Name:       "toolName",
	//	//	//	//	Content:    "textual tool response content",
	//	//	//	//},
	//	//	//	llms.TextPart("What is the number from the first message, multiplied by 5? Respond only with the number, and nothing else"),
	//	//	//},  {
	//	//	//	Role:  llms.ChatMessageTypeHuman,
	//	//	//	Parts: []llms.ContentPart{
	//	//	//	llms.BinaryPart("mimeType", []byte{}),
	//	//	//	llms.TextPart("What is the number from the first message, multiplied by 5? Respond only with the number, and nothing else"),
	//	//	//},
	//	//})
	//	//if err != nil {
	//	//	panic("error getting generated response: " + err.Error())
	//	//}
	//	//respBs, err := json.MarshalIndent(response, "", " ")
	//	//fmt.Printf("Agent response:\n%s\n", string(respBs))
	//	//
	//	////// Define the codingAgentToolkit the agent can use
	//	//availableTools := []codingAgentToolkit.Tool{
	//	//	SearchTool{}, // Add our mock search tool
	//	//}
	//	//
	//	////// Create a new agent executor
	//	////// The ZeroShotAgent is a common type of agent in LangChain
	//	//
	//	////
	//	////// Run the agent with a prompt

}
