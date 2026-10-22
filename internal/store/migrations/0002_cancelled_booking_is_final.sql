-- 0002: a cancelled booking is final (FR-008, INV-004, ADR-0010).
--
-- The partial unique index of 0001 only constrains active rows, so without
-- this trigger an UPDATE could set a cancelled booking active again whenever
-- its slot is free. The trigger aborts any update that moves a booking out of
-- 'cancelled'; cancelled to cancelled (a repeated cancel, ADR-0003) is allowed.
-- The store maps the abort, recognised by its message, to a stable error.

CREATE TRIGGER booking_cancelled_is_final
BEFORE UPDATE OF status ON booking
FOR EACH ROW
WHEN OLD.status = 'cancelled' AND NEW.status <> 'cancelled'
BEGIN
    SELECT RAISE(ABORT, 'booking_cancelled_is_final');
END;
