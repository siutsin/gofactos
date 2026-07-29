local surface = game.surfaces[surface_name]
local circuit_green = defines.wire_connector_id.circuit_green
local active = { RUNNING = 0, DONE = 0, ["STACK OVERFLOW"] = 0 }
local found = { RUNNING = 0, DONE = 0, ["STACK OVERFLOW"] = 0 }

for _, panel in pairs(surface.find_entities_filtered {
  name = "display-panel",
}) do
  local behavior = panel.get_control_behavior()
  local messages = behavior and behavior.messages or {}
  if #messages == 1 then
    local message = messages[1]
    local text = message.text
    if active[text] ~= nil then
      found[text] = found[text] + 1
      local condition = message.condition
      assert(condition, "status condition is missing")
      assert(
        condition.comparator == "=" and condition.constant == 1,
        "invalid status condition"
      )
      assert(condition.first_signal, "status signal is missing")
      local value = panel.get_signal(
        condition.first_signal,
        circuit_green
      )
      if value == condition.constant then
        active[text] = active[text] + 1
      end
    end
  end
end

assert(found.RUNNING == 1, "RUNNING panel is missing")
assert(found.DONE == 1, "DONE panel is missing")
assert(
  found["STACK OVERFLOW"] == 1,
  "STACK OVERFLOW panel is missing"
)
rcon.print(
  "running=" .. active.RUNNING
    .. " done=" .. active.DONE
    .. " overflow=" .. active["STACK OVERFLOW"]
)
