# Parallel Search MCP

Run a web research agent with the free, keyless [Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp). This example explicitly configures `agent.Options.MCPServers` with the HTTP endpoint `https://search.parallel.ai/mcp`. It uses the SDK's MCP discovery, tool wrapper, and agent loop to search, fetch source excerpts when needed, and answer with source URLs.

From the repository root, configure your usual model API credentials and run:

```sh
export CODEANY_API_KEY=your-model-api-key
# Set CODEANY_BASE_URL and CODEANY_MODEL for your preferred provider if needed.
go run ./examples/12-parallel-search/ "What changed in Go 1.23? Cite official sources."
```

The model API still requires its own credentials and may incur charges. Parallel's anonymous endpoint requires no Parallel key and has lower rate limits. This example sends no authentication headers to Parallel and does not read `PARALLEL_API_KEY`.

The example advertises only `mcp__parallel__web_search` and `mcp__parallel__web_fetch` to the model and uses `PermissionModeDontAsk` to restrict execution to those tools. Each conversation gets a random `session_id` in the system prompt; the agent is instructed to reuse it across search and fetch calls. Requests identify the caller as `open-agent-sdk-go/0.1.0` through `MCPServerConfig.Headers`. A three minute context deadline covers discovery and the agent loop. Discovery failures stop the example before a model request.

Running this example opts into Parallel for that process. The SDK's default tool registry, `WebSearch.SearchFn`, existing examples, and model provider selection are unchanged. To adapt the example, copy its `MCPServers` configuration into your own agent and select the prefixed tool names explicitly.

Validate locally without external credentials:

```sh
go test ./examples/12-parallel-search/ -v
```

The tests use local HTTP MCP and streaming model API fixtures to exercise discovery, the effective tool list, search and fetch execution, source content reaching the next model turn, and the final answer. To test the real anonymous MCP endpoint with the same deterministic local model API fixture:

```sh
PARALLEL_MCP_LIVE_TEST=1 go test ./examples/12-parallel-search/ -run TestResearchLive -v -count=1
```

The live check sends one search and one fetch to Parallel and requires network access, but no model API credentials. It verifies integration behavior rather than model answer quality.
