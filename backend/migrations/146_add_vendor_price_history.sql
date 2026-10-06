-- 146_add_vendor_price_history.sql
-- Stores price history snapshots for upstream AI vendors and models

CREATE TABLE IF NOT EXISTS ash_vendor_price_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id UUID NOT NULL REFERENCES ash_vendors(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    input_price NUMERIC(15, 6) NOT NULL,
    output_price NUMERIC(15, 6) NOT NULL,
    is_available BOOLEAN NOT NULL DEFAULT true,
    recorded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ash_vendor_price_history_vendor ON ash_vendor_price_history(vendor_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_ash_vendor_price_history_model ON ash_vendor_price_history(model_name, recorded_at DESC);
