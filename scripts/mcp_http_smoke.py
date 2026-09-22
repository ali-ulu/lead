#!/usr/bin/env python3
from __future__ import annotations

import asyncio

from mcp import Client


async def main() -> None:
    async with Client("http://127.0.0.1:8790/mcp") as client:
        result = await client.call_tool("capabilities", {})
        if result.is_error:
            raise SystemExit("HTTP MCP capabilities failed")
        data = result.structured_content or {}
        if data.get("name") != "LeadScout":
            raise SystemExit(f"unexpected HTTP MCP response: {data!r}")
        print("MCP HTTP PASS: connected to /mcp and called capabilities")


if __name__ == "__main__":
    asyncio.run(main())
