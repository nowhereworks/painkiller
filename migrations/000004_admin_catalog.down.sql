UPDATE purchased_tests SET stripe_session_id = '' WHERE stripe_session_id IS NULL;
ALTER TABLE purchased_tests ALTER COLUMN stripe_session_id SET NOT NULL;
ALTER TABLE products DROP COLUMN is_free;
