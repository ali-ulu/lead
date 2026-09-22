#!/usr/bin/env python3
from __future__ import annotations

import asyncio

from mcp import Client

from mcp_server import mcp


async def main() -> None:
    async with Client(mcp, raise_exceptions=True) as client:
        result = await client.call_tool("capabilities", {})
        if result.is_error:
            raise SystemExit("capabilities tool returned an error")
        data = result.structured_content or {}
        if data.get("name") != "LeadScout":
            raise SystemExit(f"unexpected MCP identity: {data!r}")

        listed = await client.call_tool("list_leads", {"page_size": 5})
        if listed.is_error:
            raise SystemExit("list_leads tool returned an error")
        payload = listed.structured_content or {}
        if "items" not in payload or "total" not in payload:
            raise SystemExit(f"unexpected list_leads result: {payload!r}")

        print("MCP PASS: capabilities and list_leads tools executed in-process")


if __name__ == "__main__":
    asyncio.run(main())
