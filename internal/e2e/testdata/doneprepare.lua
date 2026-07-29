assert(
  script.get_event_handler(defines.events.on_tick),
  "DONE monitor is missing"
)

local surface = game.surfaces[surface_name]
local clock = nil
if period > 0 then
  for _, entity in pairs(surface.find_entities_filtered {
    name = "decider-combinator",
  }) do
    local behavior = entity.get_control_behavior()
    local condition = behavior.get_condition(1)
    local output = behavior.get_output(1)
    if condition
      and output
      and condition.first_signal
      and condition.first_signal.name == "signal-dot"
      and condition.comparator == "="
      and condition.constant == 2
    then
      assert(not clock, "multiple clock pulse combinators")
      clock = entity
    end
  end
  assert(clock, "clock pulse combinator is missing")
end

storage.gofactos_e2e_done = {
  surface = surface_name,
  period = period,
  clock = clock,
  seen = false,
  display = -1,
  panels = 0,
  tick = 0,
  pulses = 0,
  last_pulse = 0,
  cadence = true,
  bad_delta = 0,
}
rcon.print("monitor=armed")
