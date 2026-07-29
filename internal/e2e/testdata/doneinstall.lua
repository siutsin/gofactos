assert(
  not script.get_event_handler(defines.events.on_tick),
  "on_tick handler already installed"
)
storage.gofactos_e2e_done = nil

script.on_event(defines.events.on_tick, function(event)
  local watch = storage.gofactos_e2e_done
  if not watch or watch.seen then
    return
  end

  local surface = game.surfaces[watch.surface]
  if not surface then
    return
  end

  if watch.clock then
    assert(watch.clock.valid, "clock pulse combinator is invalid")
    local behavior = watch.clock.get_control_behavior()
    local output = behavior.get_output(1)
    assert(output and output.signal, "clock output is missing")
    local pulse = behavior.get_signal_last_tick(output.signal) or 0
    if pulse ~= 0 then
      if watch.last_pulse > 0 then
        local delta = event.tick - watch.last_pulse
        if delta ~= watch.period then
          watch.cadence = false
          watch.bad_delta = delta
        end
      end
      watch.last_pulse = event.tick
      watch.pulses = watch.pulses + 1
    end
  end

  local output_green =
    defines.wire_connector_id.combinator_output_green
  local mode_signal = { type = "virtual", name = "signal-M" }
  local mode = 0
  for _, entity in pairs(surface.find_entities_filtered {
    name = "arithmetic-combinator",
  }) do
    mode = math.max(
      mode,
      entity.get_signal(mode_signal, output_green)
    )
  end
  if mode ~= done_mode then
    return
  end

  local circuit_red = defines.wire_connector_id.circuit_red
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
          local digit = panel.get_signal(
            digit_signal,
            circuit_red
          )
          display = display + digit * 10 ^ tonumber(place)
        end
      end
    end
  end

  watch.seen = true
  watch.display = display
  watch.panels = panels
  watch.tick = event.tick
end)

rcon.print("monitor=ready")
