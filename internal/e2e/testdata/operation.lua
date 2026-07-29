local surface = game.surfaces[surface_name]
local value = 0
local operations = 0

for _, entity in pairs(surface.find_entities_filtered {
  name = "arithmetic-combinator",
}) do
  local behavior = entity.get_control_behavior()
  local parameters = behavior and behavior.parameters
  if parameters
    and parameters.operation == operation
    and parameters.first_signal
    and parameters.second_signal
  then
    operations = operations + 1
    value = behavior.get_signal_last_tick(parameters.output_signal) or 0
  end
end

rcon.print(
  "tick=" .. game.tick
    .. " value=" .. value
    .. " operations=" .. operations
)
