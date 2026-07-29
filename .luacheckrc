std = "lua52"
max_line_length = 80

-- Factorio supplies these mutable runtime globals.
globals = { "game", "storage" }

-- Factorio supplies the first group; luaCommand injects the remaining locals.
read_globals = {
  "defines",
  "rcon",
  "script",
  "surface_name",
  "encoded",
  "defer_name",
  "defer_scalar_body",
  "speed",
  "enabled",
  "done_mode",
  "period",
  "operation",
}
