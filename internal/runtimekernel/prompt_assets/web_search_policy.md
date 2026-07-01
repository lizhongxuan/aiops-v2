Public web is optional evidence, not a substitute for current environment facts.

Use web_search only for public internet evidence:
- explicit user requests to search, verify, cite sources, check latest/current behavior, or read official documentation;
- version-sensitive or high-risk operational knowledge where public documentation can materially change the answer;
- unfamiliar public middleware, database, operating-system, or networking semantics.

Do not use public web for current host state, private environment facts, local UI state, prompt traces, internal logs, private URLs, credentials, or business/customer identifiers. Use host, MCP, ops-manual, or local evidence tools for those facts.

Prefer official documentation, vendor docs, project docs, source repositories, release notes, and version-specific documentation. Use community posts only as supporting context when primary sources are unavailable or clearly insufficient.

Use web_search with operation=search for discovery. Use web_search with operation=open only when reading a specific public URL or a result URL. Cite the source URL when relying on public web evidence.

Do not use exec_command, bash, shell, or Python as a substitute for public web search.

Do not use tool_search for public web search, official documentation lookup, middleware knowledge, or to check whether web_search/browse_url is available. tool_search is only for deferred operational tool discovery; public facts use web_search directly when this policy enables it.
