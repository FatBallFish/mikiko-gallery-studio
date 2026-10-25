import { createReferenceReservations, referenceAdmission } from './reference-admission'
const policy = { limits: { image: 2, video: 1 } }
type Ref = { id: string; type: 'image' | 'video' }
const kind = (item: Ref) => item.type
const key = (item: Ref) => item.id
const image = (id: string): Ref => ({ id, type: 'image' })
const current = [image('old')]
function assert(value: unknown, message: string) { if (!value) throw new Error(message) }
const overflow = referenceAdmission({ policy, current, incoming: [image('a'), image('b')], kind, key })
assert(!overflow.ok && overflow.merged[0] === current[0] && overflow.merged.length === 1, 'overflow must reject the entire batch and preserve existing references')
assert(referenceAdmission({ policy, current, incoming: [image('old'), image('a'), image('a')], kind, key }).added.length === 1, 'deduplicate before counting')
assert(!referenceAdmission({ policy: { limits: { image: null } }, current, incoming: [image('a')], kind, key }).ok, 'unknown capability must reject')
assert(!referenceAdmission({ policy: { limits: { image: 0 } }, current, incoming: [], kind, key }).ok, 'existing unsupported references block without deletion')
const reservations = createReferenceReservations<'image' | 'video'>()
const first = reservations.reserve(policy, current, [image('a')], kind, key)
assert(first.ok && first.token !== undefined, 'first upload reserves a slot')
assert(!reservations.reserve(policy, current, [image('b')], kind, key).ok, 'concurrent upload must not exceed the limit')
assert(referenceAdmission({ policy, current, incoming: [image('a')], reserved: reservations.pending(first.token), kind, key }).ok, 'completion excludes its own reservation')
assert(!referenceAdmission({ policy: { limits: { image: 1 } }, current, incoming: [image('a')], reserved: reservations.pending(first.token), kind, key }).ok, 'completion revalidates reduced capability')
reservations.release(first.token!)
assert(reservations.pending().image === undefined, 'release frees reservation')
assert(referenceAdmission({ policy, current, incoming: [{ id: 'v', type: 'video' }], kind, key }).ok, 'media types have independent limits')
