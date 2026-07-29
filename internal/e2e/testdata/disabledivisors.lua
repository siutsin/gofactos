local surface = game.surfaces[surface_name]
local divisors = {}
local operations = 0

for _, entity in pairs(surface.find_entities_filtered {
  name = "arithmetic-combinator",
}) do
  local behavior = entity.get_control_behavior()
  local parameters = behavior and behavior.parameters
  if parameters
    and parameters.second_signal
    and (parameters.operation == "/" or parameters.operation == "%")
  then
    operations = operations + 1
    table.insert(divisors, parameters.second_signal)
  end
end

assert(operations == 2, "source arithmetic operations are missing")

local constants = surface.find_entities_filtered {
  name = "constant-combinator",
}

local function emits(entity, signal)
  local behavior = entity.get_control_behavior()
  for _, section in pairs(behavior.sections) do
    for _, filter in pairs(section.filters) do
      local value = filter.value
      if type(value) == "string" and value == signal.name then
        return true
      end
      if type(value) == "table"
        and value.name == signal.name
        and (
          not signal.type
          or not value.type
          or value.type == signal.type
        )
      then
        return true
      end
    end
  end
  return false
end

local sources = {}
for _, divisor in pairs(divisors) do
  local source = nil
  for _, entity in pairs(constants) do
    if emits(entity, divisor) then
      assert(not source, "multiple divisor sources")
      source = entity
    end
  end
  assert(source, "divisor source is missing")
  table.insert(sources, source)
end

for _, source in pairs(sources) do
  source.get_control_behavior().enabled = false
end

rcon.print("divisor_sources=" .. #sources)
