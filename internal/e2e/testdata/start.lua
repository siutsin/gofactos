storage.gofactos_e2e_starts = storage.gofactos_e2e_starts or {}
local surface = game.surfaces[surface_name]
local start = storage.gofactos_e2e_starts[surface_name]
if start and not start.valid then
  start = nil
end

if enabled then
  start = nil
  local constants = surface.find_entities_filtered({
    name = "constant-combinator",
  })
  for _, entity in pairs(constants) do
    if not entity.get_control_behavior().enabled then
      assert(not start, "multiple disabled constants")
      start = entity
    end
  end
  storage.gofactos_e2e_starts[surface_name] = start
end

assert(start, "START control is missing")
assert(start.valid, "START control is invalid")
start.get_control_behavior().enabled = enabled
rcon.print("start=" .. (enabled and "on" or "off"))
