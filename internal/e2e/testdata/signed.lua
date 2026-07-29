local surface = game.surfaces[surface_name]
local division = 0
local divisions = 0
local remainder = 0
local remainders = 0

for _, entity in pairs(surface.find_entities_filtered {
  name = "arithmetic-combinator",
}) do
  local behavior = entity.get_control_behavior()
  local parameters = behavior and behavior.parameters
  if parameters and parameters.second_signal then
    if parameters.operation == "/" then
      divisions = divisions + 1
      division = behavior.get_signal_last_tick(
        parameters.output_signal
      ) or 0
    elseif parameters.operation == "%" then
      remainders = remainders + 1
      remainder = behavior.get_signal_last_tick(
        parameters.output_signal
      ) or 0
    end
  end
end

rcon.print(
  "tick=" .. game.tick
    .. " division=" .. division
    .. " divisions=" .. divisions
    .. " remainder=" .. remainder
    .. " remainders=" .. remainders
)
