import React from 'react';
import './WorkflowPipeline.css';

export interface WorkflowStep {
  id: string;
  label: string;
  status: 'done' | 'active' | 'pending' | 'error';
  detail?: string;
  action?: {
    label: string;
    onClick: () => void;
    disabled?: boolean;
  };
}

interface WorkflowPipelineProps {
  steps: WorkflowStep[];
}

const stepIcon = (status: WorkflowStep['status'], index: number): React.ReactNode => {
  if (status === 'done') return '✓';
  if (status === 'error') return '✕';
  return String(index + 1);
};

const WorkflowPipeline: React.FC<WorkflowPipelineProps> = ({ steps }) => {
  return (
    <div className="workflow-pipeline">
      <div className="workflow-track">
        {steps.map((step, i) => (
          <React.Fragment key={step.id}>
            {i > 0 && (
              <div className={`workflow-connector${steps[i - 1].status === 'done' ? ' done' : ''}`} />
            )}
            <div className={`workflow-circle ${step.status}`}>
              {stepIcon(step.status, i)}
            </div>
          </React.Fragment>
        ))}
      </div>
      <div className="workflow-labels">
        {steps.map(step => (
          <div key={step.id} className={`workflow-step-info ${step.status}`}>
            <div className="workflow-step-label">{step.label}</div>
            {step.detail && <div className="workflow-step-detail">{step.detail}</div>}
            {step.action && (
              <div className="workflow-step-action">
                <button onClick={step.action.onClick} disabled={step.action.disabled}>
                  {step.action.label}
                </button>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

export default WorkflowPipeline;
