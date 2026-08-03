CREATE TABLE IF NOT EXISTS ash_vendors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    payment_methods JSONB NOT NULL DEFAULT '[]',
    currency_type TEXT NOT NULL DEFAULT 'USD',
    exchange_rate NUMERIC(15, 6) NOT NULL DEFAULT 1.0,
    status TEXT NOT NULL DEFAULT 'active',
    last_scraped_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ash_vendor_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id UUID NOT NULL REFERENCES ash_vendors(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    input_price NUMERIC(15, 6) NOT NULL,
    output_price NUMERIC(15, 6) NOT NULL,
    is_available BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (vendor_id, model_name)
);

CREATE INDEX IF NOT EXISTS idx_ash_vendor_models_vendor_id ON ash_vendor_models(vendor_id);

