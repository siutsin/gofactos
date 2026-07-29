local surface = game.surfaces[surface_name]
local circuit_green = defines.wire_connector_id.circuit_green

local function icon_name(icon)
  if type(icon) == "string" then
    return icon
  end
  if not icon then
    return nil
  end
  if icon.name then
    return icon.name
  end
  if icon.signal then
    return icon.signal.name
  end
  return nil
end

local panels = 0
local value = 0
local check_visible = 0
local deny_visible = 0
for _, panel in pairs(surface.find_entities_filtered {
  name = "display-panel",
}) do
  local behavior = panel.get_control_behavior()
  local messages = behavior and behavior.messages or {}
  if #messages == 2 then
    local by_icon = {}
    for _, message in pairs(messages) do
      local name = icon_name(message.icon)
      if name then
        by_icon[name] = message
      end
    end
    local check = by_icon["signal-check"]
    local deny = by_icon["signal-deny"]
    if check and deny then
      panels = panels + 1
      local check_condition = check.condition
      local deny_condition = deny.condition
      assert(check_condition, "check condition is missing")
      assert(deny_condition, "deny condition is missing")
      assert(
        check_condition.comparator == "="
          and check_condition.constant == 1,
        "invalid check condition"
      )
      assert(
        deny_condition.comparator == "="
          and deny_condition.constant == -1,
        "invalid deny condition"
      )
      assert(
        check_condition.first_signal
          and deny_condition.first_signal,
        "Boolean signals are missing"
      )
      assert(
        check_condition.first_signal.name
          == deny_condition.first_signal.name,
        "Boolean signals differ"
      )
      local check_value = panel.get_signal(
        check_condition.first_signal,
        circuit_green
      )
      local deny_value = panel.get_signal(
        deny_condition.first_signal,
        circuit_green
      )
      assert(check_value == deny_value, "Boolean readings differ")
      value = check_value
      if value == check_condition.constant then
        check_visible = check_visible + 1
      end
      if value == deny_condition.constant then
        deny_visible = deny_visible + 1
      end
    end
  end
end

assert(panels == 1, "expected one Boolean display")
rcon.print(
  "tick=" .. game.tick
    .. " value=" .. value
    .. " panels=" .. panels
    .. " check=" .. check_visible
    .. " deny=" .. deny_visible
)
