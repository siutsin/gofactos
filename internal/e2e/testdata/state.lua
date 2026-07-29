local surface = game.surfaces[surface_name]
local output_green = defines.wire_connector_id.combinator_output_green
local output_red = defines.wire_connector_id.combinator_output_red
local circuit_red = defines.wire_connector_id.circuit_red

local function signal(kind, name)
  if kind == "item" then
    return { type = kind, name = name, quality = "normal" }
  end
  return { type = kind, name = name }
end

local function observation(entity, kind, name, connector)
  local result
  local nonzero = 0
  for _, value in pairs(surface.find_entities_filtered { name = entity }) do
    local current = value.get_signal(signal(kind, name), connector)
    if current ~= 0 then
      result = math.max(result or current, current)
      nonzero = nonzero + 1
    end
  end
  return result or 0, nonzero
end

local mode, mode_nonzero = observation(
  "arithmetic-combinator",
  "virtual",
  "signal-M",
  output_green
)
local result, result_nonzero = observation(
  "arithmetic-combinator",
  "virtual",
  "signal-O",
  output_green
)
local output, output_nonzero = observation(
  "arithmetic-combinator",
  "item",
  "iron-plate",
  output_green
)

local function private_state_nonzero()
  local count = 0
  local names = {
    "signal-S",
    "signal-R",
    "signal-P",
    "signal-0",
    "signal-1",
    "signal-2",
    "signal-3",
    "signal-4",
    "signal-5",
    "signal-6",
    "signal-7",
    "signal-8",
    "signal-9",
  }
  for _, entity_name in pairs {
    "arithmetic-combinator",
    "decider-combinator",
  } do
    for _, entity in pairs(surface.find_entities_filtered {
      name = entity_name,
    }) do
      for _, connector in pairs { output_green, output_red } do
        for _, name in pairs(names) do
          if entity.get_signal(
            signal("virtual", name),
            connector
          ) ~= 0 then
            count = count + 1
          end
        end
      end
    end
  end
  return count
end

local stack = private_state_nonzero()

local function read_display()
  local display = 0
  local panels = 0
  for _, panel in pairs(surface.find_entities_filtered {
    name = "display-panel",
  }) do
    local behavior = panel.get_control_behavior()
    if behavior then
      local messages = behavior.messages
      if #messages == 10 then
        local digit_signal = messages[1].condition.first_signal
        local place = string.match(
          digit_signal.name,
          "^signal%-(%d)$"
        )
        if place then
          panels = panels + 1
          local digit = panel.get_signal(digit_signal, circuit_red)
          display = display + digit * 10 ^ tonumber(place)
        end
      end
    end
  end
  return display, panels
end

local display, panels = read_display()
local activity = 0
for _, lamp in pairs(surface.find_entities_filtered { name = "small-lamp" }) do
  if not lamp.get_control_behavior().disabled then
    activity = activity + 1
  end
end

local disabled = 0
for _, entity in pairs(surface.find_entities_filtered {
  name = "constant-combinator",
}) do
  if not entity.get_control_behavior().enabled then
    disabled = disabled + 1
  end
end

rcon.print(
  "tick=" .. game.tick
    .. " mode=" .. mode
    .. " result=" .. result
    .. " output=" .. output
    .. " display=" .. display
    .. " activity=" .. activity
    .. " mode_signals=" .. mode_nonzero
    .. " result_signals=" .. result_nonzero
    .. " output_signals=" .. output_nonzero
    .. " stack=" .. stack
    .. " panels=" .. panels
    .. " ghosts=" .. #surface.find_entities_filtered {
      type = "entity-ghost",
    }
    .. " disabled=" .. disabled
)
