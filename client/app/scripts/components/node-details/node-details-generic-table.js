import React from 'react';
import { Map as makeMap } from 'immutable';
import map from 'lodash/map';

import MatchedText from '../matched-text';
import ShowMore from '../show-more';

export default class NodeDetailsGenericTable extends React.Component {

  constructor(props, context) {
    super(props, context);
    this.DEFAULT_LIMIT = 5;
    this.state = {
      limit: this.DEFAULT_LIMIT,
    };
    this.handleLimitClick = this.handleLimitClick.bind(this);
  }

  handleLimitClick() {
    const limit = this.state.limit ? 0 : this.DEFAULT_LIMIT;
    this.setState({limit});
  }

  render() {
    const { matches = makeMap() } = this.props;
    let rows = this.props.rows;
    let notShown = 0;
    const limited = rows && this.state.limit > 0 && rows.length > this.state.limit;
    const expanded = this.state.limit === 0;
    if (rows && limited) {
      const hasNotShownMatch = rows.filter((row, index) => index >= this.state.limit
        && matches.has(row.id)).length > 0;
      if (!hasNotShownMatch) {
        notShown = rows.length - this.DEFAULT_LIMIT;
        rows = rows.slice(0, this.state.limit);
      }
    }

    return (
      <table className="node-details-generic-table">
        {rows.map(row => (
          <tr className="node-details-generic-table-row" key={row.id}>
            {map(row.entries, (value, column) => (
              <td className="node-details-generic-table-field-value truncate" title={value}>
                <MatchedText text={value} match={matches.get(column)} />
              </td>
            ))}
          </tr>
        ))}
        <ShowMore
          handleClick={this.handleLimitClick} collection={this.props.rows}
          expanded={expanded} notShown={notShown}
        />
      </table>
    );
  }
}
