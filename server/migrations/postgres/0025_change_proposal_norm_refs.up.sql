ALTER TABLE agent_change_proposals
    ADD COLUMN norm_refs jsonb NOT NULL DEFAULT '[]'::jsonb;
