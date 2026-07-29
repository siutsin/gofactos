if game.surfaces[surface_name] then
  game.delete_surface(surface_name)
end

local surface = game.create_surface(surface_name, {
  width = 256,
  height = 256,
})
surface.always_day = true
surface.generate_with_lab_tiles = true
surface.request_to_generate_chunks({0, 0}, 3)
surface.force_generate_chunk_requests()

local inventory = game.create_inventory(1)
local stack = inventory[1]
stack.set_stack({name = "blueprint", count = 1})
stack.import_stack(encoded)

local target = nil
local anchor = nil
if defer_scalar_body then
  local additions = 0
  for _, spec in pairs(stack.get_blueprint_entities()) do
    if spec.name == "substation" then
      assert(not anchor, "multiple blueprint substations")
      anchor = {x = spec.position.x, y = spec.position.y}
    end
    if spec.name == "arithmetic-combinator" then
      local behavior = spec.control_behavior
      local conditions = behavior and behavior.arithmetic_conditions
      if conditions and conditions.operation == "+" then
        additions = additions + 1
        if not target or spec.position.y > target.y then
          target = {x = spec.position.x, y = spec.position.y}
        end
      end
    end
  end
  assert(additions == 2, "expected two scalar additions")
  assert(target, "scalar body adder is missing")
  assert(anchor, "blueprint substation is missing")
end

stack.build_blueprint({
  surface = surface,
  force = game.forces.player,
  position = {0, 0},
  force_build = true,
  skip_fog_of_war = true,
})

if defer_scalar_body then
  local anchor_ghost = nil
  local ghosts = surface.find_entities_filtered({type = "entity-ghost"})
  for _, ghost in pairs(ghosts) do
    if ghost.ghost_name == "substation" then
      assert(not anchor_ghost, "multiple substation ghosts")
      anchor_ghost = ghost
    end
  end
  assert(anchor_ghost, "substation ghost is missing")
  target.x = target.x + anchor_ghost.position.x - anchor.x
  target.y = target.y + anchor_ghost.position.y - anchor.y
end

inventory.destroy()
local deferred = 0
local ghosts = surface.find_entities_filtered({type = "entity-ghost"})
for _, ghost in pairs(ghosts) do
  local should_defer = ghost.ghost_name == defer_name
  if defer_scalar_body then
    should_defer = ghost.ghost_name == "arithmetic-combinator"
      and math.abs(ghost.position.x - target.x) < 0.01
      and math.abs(ghost.position.y - target.y) < 0.01
  end
  if should_defer then
    deferred = deferred + 1
  else
    local _, entity = ghost.revive({raise_revive = false})
    assert(entity, "failed to revive initial ghost")
  end
end

local substations = surface.find_entities_filtered({name = "substation"})
assert(#substations > 0, "blueprint has no substation")
local substation_position = substations[1].position
local source_position = nil
for radius = 3, 8 do
  for dx = -radius, radius do
    for dy = -radius, radius do
      if not source_position
        and (math.abs(dx) == radius or math.abs(dy) == radius)
      then
        local candidate = {
          x = substation_position.x + dx,
          y = substation_position.y + dy,
        }
        local area = {
          {candidate.x - 1.1, candidate.y - 1.1},
          {candidate.x + 1.1, candidate.y + 1.1},
        }
        if #surface.find_entities_filtered({area = area}) == 0 then
          source_position = candidate
        end
      end
    end
  end
end
assert(source_position, "no power source position")

local ghosts_before = #surface.find_entities_filtered({type = "entity-ghost"})
local source = surface.create_entity({
  name = "electric-energy-interface",
  position = source_position,
  force = game.forces.player,
})
assert(source, "failed to create power source")
assert(
  ghosts_before == #surface.find_entities_filtered({type = "entity-ghost"}),
  "power source replaced a blueprint ghost"
)
source.power_production = 10000000000

rcon.print(
  "deferred=" .. deferred
    .. " remaining="
    .. #surface.find_entities_filtered({type = "entity-ghost"})
)
