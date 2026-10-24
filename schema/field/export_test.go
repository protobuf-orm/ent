// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package field

// Implements is RType's implements, which is not exported (see there), for
// the tests in field_test.
var Implements = (*RType).implements
